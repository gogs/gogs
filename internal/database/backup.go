package database

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	jsoniter "github.com/json-iterator/go"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osutil"
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

	err = dumpLegacyTables(ctx, dirPath, verbose)
	if err != nil {
		return errors.Wrap(err, "dump legacy tables")
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

		err = jsoniter.NewEncoder(w).Encode(elem)
		if err != nil {
			return errors.Wrap(err, "encode JSON")
		}
	}
	return rows.Err()
}

func dumpLegacyTables(ctx context.Context, dirPath string, verbose bool) error {
	// Purposely create a local variable to not modify global variable
	legacyTables := append(legacyTables, new(Version))
	for _, table := range legacyTables {
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
				return errors.Wrap(err, "create JSON file")
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

// ImportDatabase imports data from backup archive in JSON Lines format.
func ImportDatabase(ctx context.Context, db *gorm.DB, dirPath string, verbose bool) error {
	err := importLegacyTables(ctx, dirPath, verbose)
	if err != nil {
		return errors.Wrap(err, "import legacy tables")
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
			if !osutil.IsFile(tableFile) {
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

func importTable(ctx context.Context, db *gorm.DB, table any, r io.Reader) error {
	err := db.WithContext(ctx).Migrator().DropTable(table)
	if err != nil {
		return errors.Wrap(err, "drop table")
	}

	err = db.WithContext(ctx).Migrator().AutoMigrate(table)
	if err != nil {
		return errors.Wrap(err, "auto migrate")
	}

	s, err := schema.Parse(table, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		return errors.Wrap(err, "parse schema")
	}
	rawTableName := s.Table
	skipResetIDSeq := map[string]bool{
		"lfs_object": true,
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		// PostgreSQL does not like the null characters (U+0000)
		cleaned := bytes.ReplaceAll(scanner.Bytes(), []byte("\\u0000"), []byte(""))

		elem := reflect.New(reflect.TypeOf(table).Elem()).Interface()
		err = jsoniter.Unmarshal(cleaned, elem)
		if err != nil {
			return errors.Wrap(err, "unmarshal JSON to struct")
		}

		err = db.WithContext(ctx).Create(elem).Error
		if err != nil {
			return errors.Wrap(err, "create row")
		}
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

func importLegacyTables(ctx context.Context, dirPath string, verbose bool) error {
	skipInsertProcessors := map[string]bool{
		"mirror":    true,
		"milestone": true,
	}

	// Purposely create a local variable to not modify global variable
	legacyTables := append(legacyTables, new(Version))
	for _, table := range legacyTables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tableName := strings.TrimPrefix(fmt.Sprintf("%T", table), "*database.")
		tableFile := filepath.Join(dirPath, tableName+".json")
		if !osutil.IsFile(tableFile) {
			continue
		}

		if verbose {
			log.Trace("Importing table %q...", tableName)
		}

		if err := db.WithContext(ctx).Migrator().DropTable(table); err != nil {
			return errors.Newf("drop table %q: %v", tableName, err)
		} else if err = db.WithContext(ctx).Migrator().AutoMigrate(table); err != nil {
			return errors.Newf("sync table %q: %v", tableName, err)
		}

		f, err := os.Open(tableFile)
		if err != nil {
			return errors.Newf("open JSON file: %v", err)
		}

		s, err := schema.Parse(table, &sync.Map{}, db.NamingStrategy)
		if err != nil {
			_ = f.Close()
			return errors.Wrap(err, "parse schema")
		}
		rawTableName := s.Table

		_, isInsertProcessor := table.(interface{ BeforeCreate(*gorm.DB) error })
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			// PostgreSQL does not like the null characters (U+0000)
			cleaned := bytes.ReplaceAll(scanner.Bytes(), []byte("\\u0000"), []byte(""))

			if err = jsoniter.Unmarshal(cleaned, table); err != nil {
				_ = f.Close()
				return errors.Newf("unmarshal to struct: %v", err)
			}

			if err = db.WithContext(ctx).Create(table).Error; err != nil {
				_ = f.Close()
				return errors.Newf("insert struct: %v", err)
			}

			var meta struct {
				ID             int64
				CreatedUnix    int64
				DeadlineUnix   int64
				ClosedDateUnix int64
			}
			if err = jsoniter.Unmarshal(cleaned, &meta); err != nil {
				log.Error("Failed to unmarshal to map: %v", err)
			}

			// Reset created_unix back to the date saved in archive because Create method updates its value
			if isInsertProcessor && !skipInsertProcessors[rawTableName] {
				if err = db.WithContext(ctx).Exec("UPDATE `"+rawTableName+"` SET created_unix=? WHERE id=?", meta.CreatedUnix, meta.ID).Error; err != nil {
					log.Error("Failed to reset '%s.created_unix': %v", rawTableName, err)
				}
			}

			switch rawTableName {
			case "milestone":
				if err = db.WithContext(ctx).Exec("UPDATE `"+rawTableName+"` SET deadline_unix=?, closed_date_unix=? WHERE id=?", meta.DeadlineUnix, meta.ClosedDateUnix, meta.ID).Error; err != nil {
					log.Error("Failed to reset 'milestone.deadline_unix', 'milestone.closed_date_unix': %v", err)
				}
			}
		}
		_ = f.Close()

		// PostgreSQL needs manually reset table sequence for auto increment keys
		if conf.UsePostgreSQL {
			seqName := rawTableName + "_id_seq"
			if err = db.WithContext(ctx).Exec(fmt.Sprintf(`SELECT setval('%s', COALESCE((SELECT MAX(id)+1 FROM "%s"), 1), false);`, seqName, rawTableName)).Error; err != nil {
				return errors.Newf("reset table %q' sequence: %v", rawTableName, err)
			}
		}
	}
	return nil
}
