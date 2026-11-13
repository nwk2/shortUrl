package store

import (
	"context"
	"errors"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"shortUrl/internal/model"
)

// ErrNotFound is returned when no row matches the requested hash.
var ErrNotFound = errors.New("short url not found")

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
func Open(dbPath, logLevel string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(parseLogLevel(logLevel)),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.ShortUrl{}); err != nil {
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

func (s *Store) Create(ctx context.Context, link *model.ShortUrl) error {
	return s.db.WithContext(ctx).Create(link).Error
}

func (s *Store) GetByHash(ctx context.Context, hashKey string) (model.ShortUrl, error) {
	var link model.ShortUrl
	err := s.db.WithContext(ctx).Where("hash_key = ?", hashKey).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ShortUrl{}, ErrNotFound
	}
	return link, err
}

func (s *Store) List(ctx context.Context) ([]model.ShortUrl, error) {
	var links []model.ShortUrl
	err := s.db.WithContext(ctx).Find(&links).Error
	return links, err
}

// UpdateExpiry moves a link's expiry date. Used by tests to build an
// expired link without reaching around the Store.
func (s *Store) UpdateExpiry(ctx context.Context, hashKey string, expiry time.Time) error {
	return s.db.WithContext(ctx).Model(&model.ShortUrl{}).Where("hash_key = ?", hashKey).Update("expiry_date", expiry).Error
}
