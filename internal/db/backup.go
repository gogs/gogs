package db

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	log "unknwon.dev/clog/v2"
	"xorm.io/core"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osutil"
)

// getTableType returns the type name of a table definition without package name,
// e.g. *db.LFSObject -> LFSObject.
func getTableType(t interface{}) string {
	return strings.TrimPrefix(fmt.Sprintf("%T", t), "*db.")
}

// DumpDatabase dumps all data from database to file system in JSON Lines format.
func DumpDatabase(db *gorm.DB, dirPath string, verbose bool) error {
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}

	err = dumpLegacyTables(dirPath, verbose)
	if err != nil {
		return errors.Wrap(err, "dump legacy tables")
	}

	for _, table := range Tables {
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

			return dumpTable(db, table, f)
		}()
		if err != nil {
			return errors.Wrapf(err, "dump table %q", tableName)
		}
	}

	return nil
}

func dumpTable(db *gorm.DB, table interface{}, w io.Writer) error {
	query := db.Model(table).Order("id ASC")
	switch table.(type) {
	case *LFSObject:
		query = db.Model(table).Order("repo_id, oid ASC")
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

func dumpLegacyTables(dirPath string, verbose bool) error {
	// Purposely create a local variable to not modify global variable
	legacyTables := append(legacyTables, new(Version))
	for _, table := range legacyTables {
		tableName := getTableType(table)
		if verbose {
			log.Trace("Dumping table %q...", tableName)
		}

		tableFile := filepath.Join(dirPath, tableName+".json")
		f, err := os.Create(tableFile)
		if err != nil {
			return fmt.Errorf("create JSON file: %v", err)
		}

		if err = x.Asc("id").Iterate(table, func(idx int, bean interface{}) (err error) {
			return jsoniter.NewEncoder(f).Encode(bean)
		}); err != nil {
			_ = f.Close()
			return fmt.Errorf("dump table '%s': %v", tableName, err)
		}
		_ = f.Close()
	}
	return nil
}

// ImportDatabase imports data from backup archive in JSON Lines format.
func ImportDatabase(db *gorm.DB, dirPath string, verbose bool) error {
	err := importLegacyTables(dirPath, verbose)
	if err != nil {
		return errors.Wrap(err, "import legacy tables")
	}

	for _, table := range Tables {
		tableName := strings.TrimPrefix(fmt.Sprintf("%T", table), "*db.")
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

			return importTable(db, table, f)
		}()
		if err != nil {
			return errors.Wrapf(err, "import table %q", tableName)
		}
	}

	return nil
}

func importTable(db *gorm.DB, table interface{}, r io.Reader) error {
	err := db.Migrator().DropTable(table)
	if err != nil {
		return errors.Wrap(err, "drop table")
	}

	err = db.Migrator().AutoMigrate(table)
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

		err = db.Create(elem).Error
		if err != nil {
			return errors.Wrap(err, "create row")
		}
	}

	// PostgreSQL needs manually reset table sequence for auto increment keys
	if conf.UsePostgreSQL && !skipResetIDSeq[rawTableName] {
		seqName := rawTableName + "_id_seq"
		if _, err = x.Exec(fmt.Sprintf(`SELECT setval('%s', COALESCE((SELECT MAX(id)+1 FROM "%s"), 1), false);`, seqName, rawTableName)); err != nil {
			return errors.Wrapf(err, "reset table %q.%q", rawTableName, seqName)
		}
	}
	return nil
}

func importLegacyTables(dirPath string, verbose bool) error {
	snakeMapper := core.SnakeMapper{}

	skipInsertProcessors := map[string]bool{
		"mirror":    true,
		"milestone": true,
	}

	// Purposely create a local variable to not modify global variable
	legacyTables := append(legacyTables, new(Version))
	for _, table := range legacyTables {
		tableName := strings.TrimPrefix(fmt.Sprintf("%T", table), "*db.")
		tableFile := filepath.Join(dirPath, tableName+".json")
		if !osutil.IsFile(tableFile) {
			continue
		}

		if verbose {
			log.Trace("Importing table %q...", tableName)
		}

		if err := x.DropTables(table); err != nil {
			return fmt.Errorf("drop table %q: %v", tableName, err)
		} else if err = x.Sync2(table); err != nil {
			return fmt.Errorf("sync table %q: %v", tableName, err)
		}

		f, err := os.Open(tableFile)
		if err != nil {
			return fmt.Errorf("open JSON file: %v", err)
		}
		rawTableName := x.TableName(table)
		_, isInsertProcessor := table.(xorm.BeforeInsertProcessor)
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			if err = jsoniter.Unmarshal(scanner.Bytes(), table); err != nil {
				return fmt.Errorf("unmarshal to struct: %v", err)
			}

			if _, err = x.Insert(table); err != nil {
				return fmt.Errorf("insert strcut: %v", err)
			}

			var meta struct {
				ID             int64
				CreatedUnix    int64
				DeadlineUnix   int64
				ClosedDateUnix int64
			}
			if err = jsoniter.Unmarshal(scanner.Bytes(), &meta); err != nil {
				log.Error("Failed to unmarshal to map: %v", err)
			}

			// Reset created_unix back to the date save in archive because Insert method updates its value
			if isInsertProcessor && !skipInsertProcessors[rawTableName] {
				if _, err = x.Exec("UPDATE `"+rawTableName+"` SET created_unix=? WHERE id=?", meta.CreatedUnix, meta.ID); err != nil {
					log.Error("Failed to reset '%s.created_unix': %v", rawTableName, err)
				}
			}

			switch rawTableName {
			case "milestone":
				if _, err = x.Exec("UPDATE `"+rawTableName+"` SET deadline_unix=?, closed_date_unix=? WHERE id=?", meta.DeadlineUnix, meta.ClosedDateUnix, meta.ID); err != nil {
					log.Error("Failed to reset 'milestone.deadline_unix', 'milestone.closed_date_unix': %v", err)
				}
			}
		}

		// PostgreSQL needs manually reset table sequence for auto increment keys
		if conf.UsePostgreSQL {
			rawTableName := snakeMapper.Obj2Table(tableName)
			seqName := rawTableName + "_id_seq"
			if _, err = x.Exec(fmt.Sprintf(`SELECT setval('%s', COALESCE((SELECT MAX(id)+1 FROM "%s"), 1), false);`, seqName, rawTableName)); err != nil {
				return fmt.Errorf("reset table %q' sequence: %v", rawTableName, err)
			}
		}
	}
	return nil
}
