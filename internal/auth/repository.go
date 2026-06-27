package auth

import "context"

type UserRepository interface {
	CreateUser(ctx context.Context, username string, passwordHash []byte) (User, error)
	FindByUsername(ctx context.Context, username string) (User, error)
	IncrementFailedAttempts(ctx context.Context, username string, lockedUntil *TimePtr) error
	ResetFailedAttempts(ctx context.Context, username string) error
	SetLastLogin(ctx context.Context, username string, at TimePtr) error
	SetTOTP(ctx context.Context, username, secret string, enabled bool) error
}

// TimePtr is a small adapter that lets the repository distinguish nil from a zero time.
type TimePtr struct {
	TimeUnixNano int64
}
