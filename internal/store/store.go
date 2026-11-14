package store

import (
	"context"
	"errors"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"shorturl/internal/model"
)

// ErrNotFound is returned when no row matches the requested hash.
var ErrNotFound = errors.New("short url not found")

// ErrHashConflict is returned when an insert fails because the hash key
// (and, derived from it, the short URL) is already taken. The service
// retries with a fresh hash instead of surfacing a 500.
var ErrHashConflict = errors.New("hash key already exists")

// Store owns all persistence for short links. It wraps a single *gorm.DB;
// callers never touch the database directly, which is what lets tests
// build their own Store on an in-memory database instead of overwriting
// a package-level global.
type Store struct {
	db *gorm.DB
}

// Open connects to the SQLite file at dbPath (created if missing) and
// runs AutoMigrate. logLevel is one of info, warn, error, silent;
// anything else falls back to info.
//
// The DSN enables WAL mode and a 5s busy timeout. WAL lets readers run
// concurrently with the writer; the busy timeout makes a write that
// lands while the database is locked wait instead of failing at once
// with "database is locked" (the default timeout is 0). Without it, a
// burst of parallel creates could 500. WAL keeps -wal and -shm sidecar
// files next to the database — a file copy for backup must include them,
// or checkpoint first.
func Open(dbPath, logLevel string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(parseLogLevel(logLevel)),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.ShortURL{}); err != nil {
		return nil, err
	}
	return New(db), nil
}

// New wraps an already-open database. Tests use this to point a Store at
// an in-memory database they control.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

func parseLogLevel(s string) logger.LogLevel {
	switch s {
	case "warn":
		return logger.Warn
	case "error":
		return logger.Error
	case "silent":
		return logger.Silent
	default:
		return logger.Info
	}
}

func (s *Store) Create(ctx context.Context, link *model.ShortURL) error {
	err := s.db.WithContext(ctx).Create(link).Error
	if err != nil && isUniqueViolation(err) {
		return ErrHashConflict
	}
	return err
}

// isUniqueViolation reports whether err is a SQLite unique-constraint
// failure. The driver surfaces it as a typed sqlite3.Error carrying the
// extended code ErrConstraintUnique; anything else (a locked database,
// a disk error) is a real failure, not a collision.
func isUniqueViolation(err error) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
	}
	return false
}

func (s *Store) GetByHash(ctx context.Context, hashKey string) (model.ShortURL, error) {
	var link model.ShortURL
	err := s.db.WithContext(ctx).Where("hash_key = ?", hashKey).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ShortURL{}, ErrNotFound
	}
	return link, err
}

func (s *Store) List(ctx context.Context, limit, offset int) ([]model.ShortURL, error) {
	var links []model.ShortURL
	// Only live links, in creation order so pagination is stable.
	err := s.db.WithContext(ctx).Where("expiry_date >= ?", time.Now()).Order("id ASC").Limit(limit).Offset(offset).Find(&links).Error
	return links, err
}

// DeleteExpired removes expired links, returning how many rows were
// removed. The hourly sweeper in main calls this; it is also what lets
// tests prove an expired link is really gone.
func (s *Store) DeleteExpired(ctx context.Context) (int64, error) {
	res := s.db.WithContext(ctx).Where("expiry_date < ?", time.Now()).Delete(&model.ShortURL{})
	return res.RowsAffected, res.Error
}

// UpdateExpiry moves a link's expiry date. Used by tests to build an
// expired link without reaching around the Store.
func (s *Store) UpdateExpiry(ctx context.Context, hashKey string, expiry time.Time) error {
	return s.db.WithContext(ctx).Model(&model.ShortURL{}).Where("hash_key = ?", hashKey).Update("expiry_date", expiry).Error
}
