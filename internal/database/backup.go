package database

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/osx"
)

// getTableType returns the type name of a table definition without package name,
// e.g., *database.LFSObject -> LFSObject.
func getTableType(t any) string {
	return strings.TrimPrefix(fmt.Sprintf("%T", t), "*database.")
}

// DumpDatabase dumps all data from database to file system in JSON Lines format.
func DumpDatabase(ctx context.Context, logger *logx.Logger, db *gorm.DB, dirPath string, verbose bool) error {
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}

	for _, table := range Tables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tableName := getTableType(table)
		if verbose {
			logger.Debug("Dumping table", "tableName", tableName)
		}

		err := func() error {
			tableFile := filepath.Join(dirPath, tableName+".json")
			f, err := os.Create(tableFile)
			if err != nil {
				return errors.Wrap(err, "create table file")
			}
			defer func() { _ = f.Close() }()

			return dumpTable(ctx, db, table, f)
		}()
		if err != nil {
			return errors.Wrapf(err, "dump table %q", tableName)
		}
	}

	return nil
}

func dumpTable(ctx context.Context, db *gorm.DB, table any, w io.Writer) error {
	query := db.WithContext(ctx).Model(table)
	switch table.(type) {
	case *LFSObject:
		query = query.Order("repo_id, oid ASC")
	default:
		query = query.Order("id ASC")
	}

	rows, err := query.Rows()
	if err != nil {
		return errors.Wrap(err, "select rows")
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		elem := reflect.New(reflect.TypeOf(table).Elem()).Interface()
		err = db.ScanRows(rows, elem)
		if err != nil {
			return errors.Wrap(err, "scan rows")
		}

		switch e := elem.(type) {
		case *LFSObject:
			e.CreatedAt = e.CreatedAt.UTC()
		}

		err = json.NewEncoder(w).Encode(elem)
		if err != nil {
			return errors.Wrap(err, "encode JSON")
		}
	}
	return rows.Err()
}

// ImportDatabase migrates the database to the current schema version and
// replaces the rows of each table with those in the backup archive in JSON Lines
// format.
func ImportDatabase(ctx context.Context, logger *logx.Logger, db *gorm.DB, dirPath string, verbose bool) error {
	err := migrations.Migrate(logger, db.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "migrate")
	}
	// Backups from before public keys were unique by their owners and
	// fingerprints may have duplicates, which are merged after the import.
	err = migrations.AllowDuplicatePublicKeys(db.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "allow duplicate public keys")
	}

	for _, table := range Tables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tableName := strings.TrimPrefix(fmt.Sprintf("%T", table), "*database.")
		err := func() error {
			tableFile := filepath.Join(dirPath, tableName+".json")
			if !osx.IsFile(tableFile) {
				logger.Info("Skipped table", "tableName", tableName)
				return nil
			}

			if verbose {
				logger.Debug("Importing table", "tableName", tableName)
			}

			f, err := os.Open(tableFile)
			if err != nil {
				return errors.Wrap(err, "open table file")
			}
			defer func() { _ = f.Close() }()

			return importTable(ctx, db, table, f)
		}()
		if err != nil {
			return errors.Wrapf(err, "import table %q", tableName)
		}
	}

	err = migrations.MergeDuplicatePublicKeys(db.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "merge duplicate public keys")
	}
	// Backups from before the allowlist was only for organization repositories may
	// have it enabled on user repositories.
	err = migrations.DisableProtectBranchAllowlistOfUserRepositories(db.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "disable push allowlists of user repositories")
	}
	return nil
}

// legacyBackupKeys maps, by table, the JSON keys that backups of older versions
// use to the keys of the fields that replaced them.
var legacyBackupKeys = map[string]map[string]string{
	"EmailAddress": {"UID": "UserID"},
	"IssueUser":    {"UID": "UserID"},
	"ProtectBranch": {
		"EnableWhitelist":  "EnableAllowlist",
		"WhitelistUserIDs": "AllowlistUserIDs",
		"WhitelistTeamIDs": "AllowlistTeamIDs",
	},
	"Star": {"UID": "UserID"},
	"User": {"Passwd": "Password"},
}

// decodeBackupRow decodes a row of the table in a backup into v, and returns the
// lowercased JSON keys that the row has. A field that the row does not have,
// e.g., one added after the version that made the backup, should take the
// default value of its column instead of the zero value of its type.
func decodeBackupRow(tableName string, data []byte, v any) (map[string]bool, error) {
	var fields map[string]json.RawMessage
	err := json.Unmarshal(data, &fields)
	if err != nil {
		return nil, err
	}

	renamed := make(map[string]json.RawMessage, len(fields))
	keys := make(map[string]bool, len(fields))
	for key, value := range fields {
		if newKey, ok := legacyBackupKeys[tableName][key]; ok {
			key = newKey
		}
		renamed[key] = value
		keys[strings.ToLower(key)] = true
	}

	data, err = json.Marshal(renamed)
	if err != nil {
		return nil, err
	}
	return keys, json.Unmarshal(data, v)
}

// jsonKey returns the key of the field in JSON.
func jsonKey(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "" {
		return field.Name
	}
	return name
}

func importTable(ctx context.Context, db *gorm.DB, table any, r io.Reader) error {
	err := db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(table).Error
	if err != nil {
		return errors.Wrap(err, "delete rows")
	}

	s, err := schema.Parse(table, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		return errors.Wrap(err, "parse schema")
	}
	rawTableName := s.Table
	tableName := getTableType(table)
	skipResetIDSeq := map[string]bool{
		"lfs_object": true,
	}

	scanner := bufio.NewScanner(r)
	// A row is a single line, which can exceed the default limit of 64 KiB, e.g.,
	// a long issue description.
	scanner.Buffer(nil, math.MaxInt)
	for scanner.Scan() {
		// PostgreSQL does not like the null characters (U+0000).
		cleaned := bytes.ReplaceAll(scanner.Bytes(), []byte("\\u0000"), []byte(""))

		elem := reflect.New(reflect.TypeOf(table).Elem())
		keys, err := decodeBackupRow(tableName, cleaned, elem.Interface())
		if err != nil {
			return errors.Wrap(err, "unmarshal JSON to struct")
		}

		// Backups from before access tokens stored their SHA-256 have only the SHA-1,
		// from which the migration of version 20 computes it.
		if t, ok := elem.Interface().(*AccessToken); ok && t.SHA256 == "" {
			t.SHA256 = cryptox.SHA256(t.Sha1)
			keys["sha256"] = true
		}

		// Insert column values as they are in the archive, so that neither hooks nor
		// column defaults (e.g., true for "repository.enable_wiki") change them. Only
		// the columns that the archive lacks take their defaults.
		row := make(map[string]any, len(s.DBNames))
		for _, name := range s.DBNames {
			field := s.FieldsByDBName[name]
			if keys[strings.ToLower(jsonKey(field.StructField))] {
				row[name], _ = field.ValueOf(ctx, elem.Elem())
			}
		}
		err = db.WithContext(ctx).Table(rawTableName).Create(row).Error
		if err != nil {
			return errors.Wrap(err, "create row")
		}
	}
	if err = scanner.Err(); err != nil {
		return errors.Wrap(err, "read rows")
	}

	// PostgreSQL needs manually reset table sequence for auto increment keys.
	if conf.UsePostgreSQL && !skipResetIDSeq[rawTableName] {
		seqName := rawTableName + "_id_seq"
		if err = db.WithContext(ctx).Exec(fmt.Sprintf(`SELECT setval('%s', COALESCE((SELECT MAX(id)+1 FROM "%s"), 1), false)`, seqName, rawTableName)).Error; err != nil {
			return errors.Wrapf(err, "reset table %q.%q", rawTableName, seqName)
		}
	}
	return nil
}
