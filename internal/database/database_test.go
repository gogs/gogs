package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
}
