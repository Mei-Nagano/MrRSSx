package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SQLite connection and page-cache limits.
//
// The SQLite page cache is private to each connection, so the database layer
// can hold up to maxOpenConns * sqliteCacheSizeKiB. An RSS reader is a
// read-mostly, single-writer workload, so keeping both modest bounds memory
// without starving readers: WAL mode allows concurrent readers next to the
// single writer.
const (
	// sqliteCacheSizeKiB is the per-connection page cache (SQLite's default is
	// 2048 KiB).
	sqliteCacheSizeKiB = 8000
	// maxOpenConns keeps the pool at its previous size so that concurrent
	// requests (including the default 10 concurrent feed refreshes) are not
	// serialized.
	maxOpenConns = 25
	// maxIdleConns keeps only a few connections warm so that page cache held by
	// pooled connections is released shortly after a burst of queries.
	maxIdleConns = 2
)

// DB wraps sql.DB with initialization state tracking.
type DB struct {
	*sql.DB
	ready           chan struct{}
	once            sync.Once
	articleRevision atomic.Uint64
}

// ArticleRevision changes after feed articles are successfully saved. It lets
// clients notice background refreshes that finish between progress polls.
func (db *DB) ArticleRevision() uint64 { return db.articleRevision.Load() }

// NewDB creates a new database connection with optimized settings.
func NewDB(dataSourceName string) (*DB, error) {
	// Plain filesystem paths may contain URI delimiters, especially when the
	// user chooses a custom data directory. Preserve explicit SQLite DSNs.
	if dataSourceName != ":memory:" && !strings.HasPrefix(dataSourceName, "file:") {
		dataSourceName = "file:" + strings.NewReplacer("%", "%25", "?", "%3F", "#", "%23").Replace(filepath.ToSlash(dataSourceName))
	}
	// busy_timeout prevents "database is locked" errors, WAL gives readers
	// concurrency next to the single writer, cache_size bounds the page cache,
	// and foreign_keys makes ON DELETE CASCADE work (off by default).
	// The pragma syntax differs per driver, so it comes from driver_*.go.
	pragmas := strings.Join(sqliteParams(), "&")
	if !strings.Contains(dataSourceName, "?") {
		dataSourceName += "?" + pragmas
	} else {
		dataSourceName += "&" + pragmas
	}

	db, err := sql.Open(sqliteDriver, dataSourceName)
	if err != nil {
		return nil, err
	}

	// Set connection pool limits so the page cache stays bounded (memory)
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)

	return &DB{
		DB:    db,
		ready: make(chan struct{}),
	}, nil
}

// WaitForReady blocks until the database is initialized.
func (db *DB) WaitForReady() {
	<-db.ready
}
