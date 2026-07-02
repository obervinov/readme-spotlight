// Package store persists configuration and contribution snapshots. It supports
// two backends selected by DSN scheme: SQLite (pure-Go, the default) and
// PostgreSQL. The schema is intentionally minimal and identical across both
// dialects; only parameter placeholders differ and are rewritten at run time.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/model"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const configKey = "config"

// Store is a thin persistence layer over database/sql.
type Store struct {
	db       *sql.DB
	postgres bool
}

// Snapshot is one stored collection run.
type Snapshot struct {
	ID            string
	CreatedAt     time.Time
	Contributions []model.Contribution
}

// Open connects to the backend described by dsn and applies migrations.
//
// Accepted DSNs:
//
//	sqlite:./data/spotlight.db      -> SQLite file (pure-Go driver)
//	postgres://user:pass@host/db    -> PostgreSQL (pgx)
func Open(dsn string) (*Store, error) {
	var (
		db  *sql.DB
		err error
		pg  bool
	)
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		db, err = sql.Open("pgx", dsn)
		pg = true
	case strings.HasPrefix(dsn, "sqlite:"):
		db, err = sql.Open("sqlite", strings.TrimPrefix(dsn, "sqlite:"))
	default:
		return nil, fmt.Errorf("unsupported dsn scheme: %q", dsn)
	}
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db, postgres: pg}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// rebind converts "?" placeholders to "$1, $2, ..." for PostgreSQL.
func (s *Store) rebind(q string) string {
	if !s.postgres {
		return q
	}
	var b strings.Builder
	n := 0
	for _, r := range q {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kv (
			k TEXT PRIMARY KEY,
			v TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			id            TEXT PRIMARY KEY,
			created_at    TEXT NOT NULL,
			contributions TEXT NOT NULL
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// GetConfig returns the stored config, or (Default, false) if none is stored.
func (s *Store) GetConfig() (config.Config, bool, error) {
	var v string
	err := s.db.QueryRow(s.rebind(`SELECT v FROM kv WHERE k = ?`), configKey).Scan(&v)
	if err == sql.ErrNoRows {
		return config.Default(), false, nil
	}
	if err != nil {
		return config.Config{}, false, err
	}
	var c config.Config
	if err := json.Unmarshal([]byte(v), &c); err != nil {
		return config.Config{}, false, err
	}
	return c, true, nil
}

// SetConfig persists the config, replacing any previous value.
func (s *Store) SetConfig(c config.Config) error {
	v, err := json.Marshal(c)
	if err != nil {
		return err
	}
	q := `INSERT INTO kv (k, v) VALUES (?, ?)
	      ON CONFLICT (k) DO UPDATE SET v = excluded.v`
	_, err = s.db.Exec(s.rebind(q), configKey, string(v))
	return err
}

// SaveSnapshot stores a contribution collection run and returns its ID.
func (s *Store) SaveSnapshot(at time.Time, contribs []model.Contribution) (string, error) {
	data, err := json.Marshal(contribs)
	if err != nil {
		return "", err
	}
	id := strconv.FormatInt(at.UTC().UnixNano(), 10)
	q := `INSERT INTO snapshots (id, created_at, contributions) VALUES (?, ?, ?)`
	if _, err := s.db.Exec(s.rebind(q), id, at.UTC().Format(time.RFC3339Nano), string(data)); err != nil {
		return "", err
	}
	return id, nil
}

// LatestSnapshot returns the most recent snapshot, or (nil, nil) if none exist.
func (s *Store) LatestSnapshot() (*Snapshot, error) {
	q := `SELECT id, created_at, contributions FROM snapshots ORDER BY created_at DESC LIMIT 1`
	row := s.db.QueryRow(q)
	snap, err := scanSnapshot(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return snap, err
}

// ListSnapshots returns up to limit snapshots, newest first (metadata only:
// contributions are still populated for simplicity at this scale).
func (s *Store) ListSnapshots(limit int) ([]Snapshot, error) {
	q := s.rebind(`SELECT id, created_at, contributions FROM snapshots ORDER BY created_at DESC LIMIT ?`)
	rows, err := s.db.Query(q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *snap)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSnapshot(sc scanner) (*Snapshot, error) {
	var (
		id, createdAt, data string
	)
	if err := sc.Scan(&id, &createdAt, &data); err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339Nano, createdAt)
	var contribs []model.Contribution
	if err := json.Unmarshal([]byte(data), &contribs); err != nil {
		return nil, err
	}
	return &Snapshot{ID: id, CreatedAt: t, Contributions: contribs}, nil
}
