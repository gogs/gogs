// +build go1.8

package mssql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
)

var _ driver.Pinger = &MssqlConn{}

// Ping is used to check if the remote server is avaiable and satisfies the Pinger interface.
func (c *MssqlConn) Ping(ctx context.Context) error {
	stmt := &MssqlStmt{c, `select 1;`, 0, nil}
	_, err := stmt.ExecContext(ctx, nil)
	return err
}

var _ driver.ConnBeginTx = &MssqlConn{}

// BeginTx satisfies ConnBeginTx.
func (c *MssqlConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.ReadOnly {
		return nil, errors.New("Read-only transactions are not supported")
	}

	var tdsIsolation isoLevel
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault:
		tdsIsolation = isolationUseCurrent
	case sql.LevelReadUncommitted:
		tdsIsolation = isolationReadUncommited
	case sql.LevelReadCommitted:
		tdsIsolation = isolationReadCommited
	case sql.LevelWriteCommitted:
		return nil, errors.New("LevelWriteCommitted isolation level is not supported")
	case sql.LevelRepeatableRead:
		tdsIsolation = isolationRepeatableRead
	case sql.LevelSnapshot:
		tdsIsolation = isolationSnapshot
	case sql.LevelSerializable:
		tdsIsolation = isolationSerializable
	case sql.LevelLinearizable:
		return nil, errors.New("LevelLinearizable isolation level is not supported")
	default:
		return nil, errors.New("Isolation level is not supported or unknown")
	}
	return c.begin(ctx, tdsIsolation)
}

func (c *MssqlConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.prepareContext(ctx, query)
}

func (s *MssqlStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	list := make([]namedValue, len(args))
	for i, nv := range args {
		list[i] = namedValue(nv)
	}
	return s.queryContext(ctx, list)
}

func (s *MssqlStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	list := make([]namedValue, len(args))
	for i, nv := range args {
		list[i] = namedValue(nv)
	}
	return s.exec(ctx, list)
}
