// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

func TestParsePostgreSQLHostPort(t *testing.T) {
	tests := []struct {
		info    string
		expHost string
		expPort string
	}{
		{info: "127.0.0.1:1234", expHost: "127.0.0.1", expPort: "1234"},
		{info: "127.0.0.1", expHost: "127.0.0.1", expPort: "5432"},
		{info: "[::1]:1234", expHost: "[::1]", expPort: "1234"},
		{info: "[::1]", expHost: "[::1]", expPort: "5432"},
		{info: "/tmp/pg.sock:1234", expHost: "/tmp/pg.sock", expPort: "1234"},
		{info: "/tmp/pg.sock", expHost: "/tmp/pg.sock", expPort: "5432"},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			host, port := ParsePostgreSQLHostPort(test.info)
			assert.Equal(t, test.expHost, host)
			assert.Equal(t, test.expPort, port)
		})
	}
}

func TestParseMSSQLHostPort(t *testing.T) {
	tests := []struct {
		info    string
		expHost string
		expPort string
	}{
		{info: "127.0.0.1:1234", expHost: "127.0.0.1", expPort: "1234"},
		{info: "127.0.0.1,1234", expHost: "127.0.0.1", expPort: "1234"},
		{info: "127.0.0.1", expHost: "127.0.0.1", expPort: "1433"},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			host, port := ParseMSSQLHostPort(test.info)
			assert.Equal(t, test.expHost, host)
			assert.Equal(t, test.expPort, port)
		})
	}
}

func TestNewDSN(t *testing.T) {
	t.Run("bad dialect", func(t *testing.T) {
		_, err := NewDSN(conf.DatabaseOpts{
			Type: "bad_dialect",
		})
		assert.Equal(t, "unrecognized dialect: bad_dialect", fmt.Sprintf("%v", err))
	})

	tests := []struct {
		name    string
		opts    conf.DatabaseOpts
		wantDSN string
	}{
		{
			name: "mysql: unix",
			opts: conf.DatabaseOpts{
				Type:     "mysql",
				Host:     "/tmp/mysql.sock",
				Name:     "gogs",
				User:     "gogs",
				Password: "pa$$word",
			},
			wantDSN: "gogs:pa$$word@unix(/tmp/mysql.sock)/gogs?charset=utf8mb4&parseTime=true",
		},
		{
			name: "mysql: tcp",
			opts: conf.DatabaseOpts{
				Type:     "mysql",
				Host:     "localhost:3306",
				Name:     "gogs",
				User:     "gogs",
				Password: "pa$$word",
			},
			wantDSN: "gogs:pa$$word@tcp(localhost:3306)/gogs?charset=utf8mb4&parseTime=true",
		},

		{
			name: "postgres: unix",
			opts: conf.DatabaseOpts{
				Type:     "postgres",
				Host:     "/tmp/pg.sock",
				Name:     "gogs",
				Schema:   "test",
				User:     "gogs@local",
				Password: "pa$$word",
				SSLMode:  "disable",
			},
			wantDSN: "user='gogs@local' password='pa$$word' host='/tmp/pg.sock' port='5432' dbname='gogs' sslmode='disable' search_path='test' application_name='gogs'",
		},
		{
			name: "postgres: tcp",
			opts: conf.DatabaseOpts{
				Type:     "postgres",
				Host:     "127.0.0.1",
				Name:     "gogs",
				Schema:   "test",
				User:     "gogs@local",
				Password: "pa$$word",
				SSLMode:  "disable",
			},
			wantDSN: "user='gogs@local' password='pa$$word' host='127.0.0.1' port='5432' dbname='gogs' sslmode='disable' search_path='test' application_name='gogs'",
		},

		{
			name: "mssql",
			opts: conf.DatabaseOpts{
				Type:     "mssql",
				Host:     "127.0.0.1",
				Name:     "gogs",
				User:     "gogs@local",
				Password: "pa$$word",
			},
			wantDSN: "server=127.0.0.1; port=1433; database=gogs; user id=gogs@local; password=pa$$word;",
		},

		{
			name: "sqlite3",
			opts: conf.DatabaseOpts{
				Type: "sqlite3",
				Path: "/tmp/gogs.db",
			},
			wantDSN: "file:/tmp/gogs.db?cache=shared&mode=rwc",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn, err := NewDSN(test.opts)
			require.NoError(t, err)
			assert.Equal(t, test.wantDSN, dsn)
		})
	}
}
