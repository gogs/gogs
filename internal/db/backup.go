package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jinzhu/gorm"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
)

// DumpDatabase dumps all data from database to file system in JSON format.
func DumpDatabase(db *gorm.DB, dirPath string, verbose bool) error {
	err := os.MkdirAll(dirPath, os.ModePerm)
	if err != nil {
		return err
	}

	err = dumpLegacyTables(dirPath, verbose)
	if err != nil {
		return errors.Wrap(err, "dump legacy tables")
	}

	for _, table := range tables {
		tableName := strings.TrimPrefix(fmt.Sprintf("%T", table), "*db.")
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

			var rows *sql.Rows
			switch table.(type) {
			case *LFSObject:
				rows, err = db.Model(table).Order("repo_id, oid ASC").Rows()
			default:
				rows, err = db.Model(table).Order("id ASC").Rows()
			}
			if err != nil {
				return errors.Wrap(err, "select rows")
			}
			defer func() { _ = rows.Close() }()

			for rows.Next() {
				err = db.ScanRows(rows, table)
				if err != nil {
					return errors.Wrap(err, "scan rows")
				}

				err = jsoniter.NewEncoder(f).Encode(table)
				if err != nil {
					return errors.Wrap(err, "encode JSON")
				}
			}

			return nil
		}()
		if err != nil {
			return errors.Wrapf(err, "dump table %q", tableName)
		}
	}

	return nil
}

func dumpLegacyTables(dirPath string, verbose bool) error {
	// Purposely create a local variable to not modify global variable
	legacyTables := append(legacyTables, new(Version))
	for _, table := range legacyTables {
		tableName := strings.TrimPrefix(fmt.Sprintf("%T", table), "*db.")

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
