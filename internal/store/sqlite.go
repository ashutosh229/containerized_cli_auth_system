package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"containerized_cli_auth_system/internal/auth"

	_ "modernc.org/sqlite"
)

type DB struct {
	db    *sql.DB
	users *UserStore
}

func Open(ctx context.Context, dsn, migrationsDir string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil && filepath.Dir(dsn) != "." {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(ctx, db, migrationsDir); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{db: db, users: &UserStore{db: db}}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Users() *UserStore {
	return d.users
}

func migrate(ctx context.Context, db *sql.DB, migrationsDir string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version := entry.Name()
		var exists int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		body, err := fs.ReadFile(os.DirFS(migrationsDir), version)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

type UserStore struct {
	db *sql.DB
}

func (s *UserStore) CreateUser(ctx context.Context, username string, passwordHash []byte) (auth.User, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users(username, password_hash, registered_at)
		VALUES(?, ?, ?)`, username, string(passwordHash), now)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return auth.User{}, auth.ErrDuplicateUser
		}
		return auth.User{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return auth.User{}, err
	}
	registeredAt, _ := time.Parse(time.RFC3339Nano, now)
	return auth.User{ID: id, Username: username, PasswordHash: string(passwordHash), RegisteredAt: registeredAt}, nil
}

func (s *UserStore) FindByUsername(ctx context.Context, username string) (auth.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, COALESCE(totp_secret, ''), mfa_enabled,
		       failed_attempts, locked_until, registered_at, last_login_at
		FROM users WHERE username = ?`, username)
	var user auth.User
	var lockedUntil, registeredAt, lastLogin sql.NullString
	if err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.TOTPSecret, &user.MFAEnabled,
		&user.FailedAttempts, &lockedUntil, &registeredAt, &lastLogin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.User{}, err
		}
		return auth.User{}, err
	}
	if lockedUntil.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, lockedUntil.String)
		if err != nil {
			return auth.User{}, err
		}
		user.LockedUntil = &parsed
	}
	if registeredAt.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, registeredAt.String)
		if err != nil {
			return auth.User{}, err
		}
		user.RegisteredAt = parsed
	}
	if lastLogin.Valid {
		parsed, err := time.Parse(time.RFC3339Nano, lastLogin.String)
		if err != nil {
			return auth.User{}, err
		}
		user.LastLoginAt = &parsed
	}
	return user, nil
}

func (s *UserStore) IncrementFailedAttempts(ctx context.Context, username string, lockedUntil *auth.TimePtr) error {
	if lockedUntil != nil {
		until := time.Unix(0, lockedUntil.TimeUnixNano).UTC().Format(time.RFC3339Nano)
		_, err := s.db.ExecContext(ctx, `
			UPDATE users
			SET failed_attempts = failed_attempts + 1, locked_until = ?
			WHERE username = ?`, until, username)
		return err
	}
	_, err := s.db.ExecContext(ctx, "UPDATE users SET failed_attempts = failed_attempts + 1 WHERE username = ?", username)
	return err
}

func (s *UserStore) ResetFailedAttempts(ctx context.Context, username string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE users SET failed_attempts = 0, locked_until = NULL WHERE username = ?", username)
	return err
}

func (s *UserStore) SetLastLogin(ctx context.Context, username string, at auth.TimePtr) error {
	_, err := s.db.ExecContext(ctx, "UPDATE users SET last_login_at = ? WHERE username = ?", time.Unix(0, at.TimeUnixNano).UTC().Format(time.RFC3339Nano), username)
	return err
}

func (s *UserStore) SetTOTP(ctx context.Context, username, secret string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, "UPDATE users SET totp_secret = ?, mfa_enabled = ? WHERE username = ?", secret, enabled, username)
	return err
}
