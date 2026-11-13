// Package applog holds the process logger. It exists so internal/conf can
// install the logger without importing internal/logx, which would cycle
// through internal/process.
package applog

import "gogs.io/gogs/internal/logx"

// Logger is the process logger. internal/conf replaces it during InitLogging.
// Packages log through this variable instead of a clog global.
var Logger = logx.NewNoop()
