package database

import "fmt"

// explainInitializationError distinguishes storage access failures from damage.
// It only annotates the error; it never removes or rebuilds user data.
func explainInitializationError(err error) error {
	code, ok := sqlitePrimaryCode(err)
	if !ok {
		return err
	}

	var hint string
	switch code {
	case sqliteCantOpen:
		hint = "database file could not be opened; check that the data directory is available and that the current user can access the database and create its journal files"
	case sqliteReadOnly, sqlitePerm:
		hint = "database storage is not writable; check directory and file permissions and whether the disk is mounted read-only"
	case sqliteFull:
		hint = "database storage is full; check free space on both the data and temporary-file disks"
	case sqliteCorrupt, sqliteNotADB:
		hint = "database contents could not be read as a valid SQLite database; stop MrRSS and back up the complete data directory before recovery"
	default:
		return err
	}
	return fmt.Errorf("%s (do not delete rss.db, rss.db-wal, or rss.db-shm as a troubleshooting step): %w", hint, err)
}
