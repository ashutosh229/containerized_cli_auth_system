package auth

import "time"

type User struct {
	ID             int64
	Username       string
	PasswordHash   string
	TOTPSecret     string
	MFAEnabled     bool
	FailedAttempts int
	LockedUntil    *time.Time
	RegisteredAt   time.Time
	LastLoginAt    *time.Time
}

type Session struct {
	ID        string
	User      User
	ExpiresAt time.Time
}
