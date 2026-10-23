//go:build android

package database

import (
	"errors"
	"strconv"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// Android links the cgo SQLite driver, which goes through bionic's libc.
//
// The pure-Go driver used everywhere else emits raw stat/unlink-family system
// calls from its transpiled libc. Android's seccomp policy forbids those for
// 64-bit processes — only the *at() variants are allowed — so the process dies
// with SIGSYS the moment SQLite opens the database:
//
//	Fatal signal 31 (SIGSYS), code 1 (SYS_SECCOMP), syscall 6 (lstat)
//
// arm64 happens to have no such syscall numbers, so only x86_64 was affected,
// but the safe fix is to use the platform's own libc on Android.
import _ "github.com/mattn/go-sqlite3"

const sqliteDriver = "sqlite3"

// sqliteParams uses mattn's per-option connection syntax.
func sqliteParams() []string {
	return []string{
		"_busy_timeout=5000",
		"_journal_mode=WAL",
		"_cache_size=-" + strconv.Itoa(sqliteCacheSizeKiB),
		"_synchronous=NORMAL",
		"_foreign_keys=1",
	}
}

// sqlitePrimaryCode returns SQLite's primary result code for err, with extended
// codes collapsed into the low byte (for example SQLITE_BUSY_SNAPSHOT -> 5).
func sqlitePrimaryCode(err error) (int, bool) {
	var value sqlite3.Error
	if errors.As(err, &value) {
		return int(value.Code) & 0xff, true
	}
	var pointer *sqlite3.Error
	if errors.As(err, &pointer) && pointer != nil {
		return int(pointer.Code) & 0xff, true
	}
	return 0, false
}
