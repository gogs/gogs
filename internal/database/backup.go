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
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/cryptox"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/osx"
)

// getTableType returns the type name of a table definition without package name,
// e.g. *database.LFSObject -> LFSObject.
func getTableType(t any) string {
	return strings.TrimPrefix(fmt.Sprintf("%T", t), "*database.")
}

// DumpDatabase dumps all data from database to file system in JSON Lines format.
func DumpDatabase(ctx context.Context, db *gorm.DB, dirPath string, verbose bool) error {
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
			log.Trace("Dumping table %q...", tableName)
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
func ImportDatabase(ctx context.Context, db *gorm.DB, dirPath string, verbose bool) error {
	err := migrations.Migrate(db.WithContext(ctx))
	if err != nil {
		return errors.Wrap(err, "migrate")
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
				log.Info("Skipped table %q", tableName)
				return nil
			}

			if verbose {
				log.Trace("Importing table %q...", tableName)
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

	return nil
}

// legacyKeys maps, per table, the JSON keys of fields renamed since Gogs 0.12 to
// their current names, as archives of Gogs 0.12 have the same format version.
// encoding/json already matches keys that differ only in case.
var legacyKeys = map[string]map[string]string{
	"EmailAddress": {"UID": "UserID"},
	"IssueUser":    {"UID": "UserID"},
	"Star":         {"UID": "UserID"},
	"User":         {"Passwd": "Password"},
}

func renameKeys(row []byte, renames map[string]string) ([]byte, error) {
	var fields map[string]json.RawMessage
	err := json.Unmarshal(row, &fields)
	if err != nil {
		return nil, err
	}
	for old, current := range renames {
		value, ok := fields[old]
		if !ok {
			continue
		}
		delete(fields, old)
		if _, ok := fields[current]; !ok {
			fields[current] = value
		}
	}
	return json.Marshal(fields)
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
	skipResetIDSeq := map[string]bool{
		"lfs_object": true,
	}

	renames := legacyKeys[getTableType(table)]
	scanner := bufio.NewScanner(r)
	// A row is a single line, which can exceed the default limit of 64 KiB, e.g.,
	// a long issue description.
	scanner.Buffer(nil, math.MaxInt)
	for scanner.Scan() {
		// PostgreSQL does not like the null characters (U+0000)
		cleaned := bytes.ReplaceAll(scanner.Bytes(), []byte("\\u0000"), []byte(""))
		if renames != nil {
			cleaned, err = renameKeys(cleaned, renames)
			if err != nil {
				return errors.Wrap(err, "rename legacy keys")
			}
		}

		elem := reflect.New(reflect.TypeOf(table).Elem())
		err = json.Unmarshal(cleaned, elem.Interface())
		if err != nil {
			return errors.Wrap(err, "unmarshal JSON to struct")
		}

		// Archives of Gogs 0.12 predate the "sha256" column, which migration v20
		// derives from the "sha1" column.
		if t, ok := elem.Interface().(*AccessToken); ok && t.SHA256 == "" {
			t.SHA256 = cryptox.SHA256(t.Sha1)
		}

		// Insert column values as they are in the archive, so that neither hooks nor
		// column defaults (e.g., true for "repository.enable_wiki") change them.
		row := make(map[string]any, len(s.DBNames))
		for _, name := range s.DBNames {
			row[name], _ = s.FieldsByDBName[name].ValueOf(ctx, elem.Elem())
		}
		err = db.WithContext(ctx).Table(rawTableName).Create(row).Error
		if err != nil {
			return errors.Wrap(err, "create row")
		}
	}
	if err = scanner.Err(); err != nil {
		return errors.Wrap(err, "read rows")
	}

	// PostgreSQL needs manually reset table sequence for auto increment keys
	if conf.UsePostgreSQL && !skipResetIDSeq[rawTableName] {
		seqName := rawTableName + "_id_seq"
		if err = db.WithContext(ctx).Exec(fmt.Sprintf(`SELECT setval('%s', COALESCE((SELECT MAX(id)+1 FROM "%s"), 1), false)`, seqName, rawTableName)).Error; err != nil {
			return errors.Wrapf(err, "reset table %q.%q", rawTableName, seqName)
		}
	}
	return nil
}
