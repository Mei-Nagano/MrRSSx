//go:build !android

package database

import (
	"errors"
	"strconv"

	"modernc.org/sqlite"
)

// The desktop, server and test builds use the pure-Go SQLite driver: it needs
// no C toolchain, which keeps `go build` and `go test` working on a bare
// machine.
import _ "modernc.org/sqlite"

const sqliteDriver = "sqlite"

// sqliteParams uses modernc's _pragma=name(value) connection syntax.
func sqliteParams() []string {
	return []string{
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(WAL)",
		"_pragma=cache_size(-" + strconv.Itoa(sqliteCacheSizeKiB) + ")",
		"_pragma=synchronous(NORMAL)",
		"_pragma=foreign_keys(1)",
		// Keep the real result code instead of a generic "out of memory".
		"_error_rc=1",
	}
}

// sqlitePrimaryCode returns SQLite's primary result code for err, with extended
// codes collapsed into the low byte (for example SQLITE_BUSY_SNAPSHOT -> 5).
func sqlitePrimaryCode(err error) (int, bool) {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) || sqliteErr == nil {
		return 0, false
	}
	return sqliteErr.Code() & 0xff, true
}
