package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"xorm.io/core"
	"xorm.io/xorm"
)

func TestMigrationsCreateTables(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	db := newTestDB(t, "migrationsCreateTables")
	for _, table := range Tables {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(table))

		columnTypes, err := db.Migrator().ColumnTypes(table)
		require.NoError(t, err, stmt.Schema.Table)
		columns := make(map[string]bool, len(columnTypes))
		for _, c := range columnTypes {
			columns[c.Name()] = true
		}
		for _, name := range stmt.Schema.DBNames {
			assert.True(t, columns[name], "column %q of table %q is not created by migrations", name, stmt.Schema.Table)
		}
	}

	// XORM maps the structs in legacyTables to columns by its own rules, which
	// only an XORM engine knows. The engine does not connect to the database.
	engine, err := xorm.NewEngine("sqlite3", "file::memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = engine.Close() })
	engine.SetMapper(core.GonicMapper{})
	for _, bean := range legacyTables {
		table := engine.TableInfo(bean)
		columnTypes, err := db.Migrator().ColumnTypes(table.Name)
		require.NoError(t, err, table.Name)
		columns := make(map[string]bool, len(columnTypes))
		for _, c := range columnTypes {
			columns[c.Name()] = true
		}
		for _, name := range table.ColumnsSeq() {
			assert.True(t, columns[name], "column %q of table %q is not created by migrations", name, table.Name)
		}
	}
}
