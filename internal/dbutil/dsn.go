// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbutil

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
)

// ParsePostgreSQLHostPort parses given input in various forms defined in
// https://www.postgresql.org/docs/current/static/libpq-connect.html#LIBPQ-CONNSTRING
// and returns proper host and port number.
func ParsePostgreSQLHostPort(info string) (host, port string) {
	host, port = "127.0.0.1", "5432"
	if strings.Contains(info, ":") && !strings.HasSuffix(info, "]") {
		idx := strings.LastIndex(info, ":")
		host = info[:idx]
		port = info[idx+1:]
	} else if len(info) > 0 {
		host = info
	}
	return host, port
}

// ParseMSSQLHostPort parses given input in various forms for MSSQL and returns
// proper host and port number.
func ParseMSSQLHostPort(info string) (host, port string) {
	host, port = "127.0.0.1", "1433"
	if strings.Contains(info, ":") {
		host = strings.Split(info, ":")[0]
		port = strings.Split(info, ":")[1]
	} else if strings.Contains(info, ",") {
		host = strings.Split(info, ",")[0]
		port = strings.TrimSpace(strings.Split(info, ",")[1])
	} else if len(info) > 0 {
		host = info
	}
	return host, port
}

// NewDSN takes given database options and returns parsed DSN.
func NewDSN(opts conf.DatabaseOpts) (dsn string, err error) {
	// In case the database name contains "?" with some parameters
	concate := "?"
	if strings.Contains(opts.Name, concate) {
		concate = "&"
	}

	switch opts.Type {
	case "mysql":
		if opts.Host[0] == '/' { // Looks like a unix socket
			dsn = fmt.Sprintf("%s:%s@unix(%s)/%s%scharset=utf8mb4&parseTime=true",
				opts.User, opts.Password, opts.Host, opts.Name, concate)
		} else {
			dsn = fmt.Sprintf("%s:%s@tcp(%s)/%s%scharset=utf8mb4&parseTime=true",
				opts.User, opts.Password, opts.Host, opts.Name, concate)
		}

	case "postgres":
		host, port := ParsePostgreSQLHostPort(opts.Host)
		dsn = fmt.Sprintf("user='%s' password='%s' host='%s' port='%s' dbname='%s' sslmode='%s' search_path='%s' application_name='gogs'",
			opts.User, opts.Password, host, port, opts.Name, opts.SSLMode, opts.Schema)

	case "mssql":
		host, port := ParseMSSQLHostPort(opts.Host)
		dsn = fmt.Sprintf("server=%s; port=%s; database=%s; user id=%s; password=%s;",
			host, port, opts.Name, opts.User, opts.Password)

	case "sqlite3", "sqlite":
		dsn = "file:" + opts.Path + "?cache=shared&mode=rwc"

	default:
		return "", errors.Errorf("unrecognized dialect: %s", opts.Type)
	}

	return dsn, nil
}

// OpenDB opens a new database connection encapsulated as gorm.DB using given
// database options and GORM config.
func OpenDB(opts conf.DatabaseOpts, cfg *gorm.Config) (*gorm.DB, error) {
	dsn, err := NewDSN(opts)
	if err != nil {
		return nil, errors.Wrap(err, "parse DSN")
	}

	var dialector gorm.Dialector
	switch opts.Type {
	case "mysql":
		dialector = mysql.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	case "mssql":
		dialector = sqlserver.Open(dsn)
	case "sqlite3":
		dialector = sqlite.Open(dsn)
	case "sqlite":
		dialector = sqlite.Open(dsn)
		dialector.(*sqlite.Dialector).DriverName = "sqlite"
	default:
		panic("unreachable")
	}

	return gorm.Open(dialector, cfg)
}
