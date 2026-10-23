package database

// Primary SQLite result codes the reader reacts to. These values are part of
// SQLite's stable ABI, so they are declared here rather than imported from a
// driver: the desktop/server build uses the pure-Go driver and the Android
// build uses the cgo one, and both must classify errors the same way.
const (
	sqlitePerm     = 3  // SQLITE_PERM
	sqliteBusy     = 5  // SQLITE_BUSY
	sqliteLocked   = 6  // SQLITE_LOCKED
	sqliteReadOnly = 8  // SQLITE_READONLY
	sqliteCorrupt  = 11 // SQLITE_CORRUPT
	sqliteFull     = 13 // SQLITE_FULL
	sqliteCantOpen = 14 // SQLITE_CANTOPEN
	sqliteNotADB   = 26 // SQLITE_NOTADB
)
