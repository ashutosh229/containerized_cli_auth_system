package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type memoryRepo struct {
	users map[string]User
	next  int64
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{users: map[string]User{}, next: 1}
}

func (r *memoryRepo) CreateUser(_ context.Context, username string, passwordHash []byte) (User, error) {
	if _, ok := r.users[username]; ok {
		return User{}, ErrDuplicateUser
	}
	user := User{ID: r.next, Username: username, PasswordHash: string(passwordHash), RegisteredAt: time.Now().UTC()}
	r.next++
	r.users[username] = user
	return user, nil
}

func (r *memoryRepo) FindByUsername(_ context.Context, username string) (User, error) {
	user, ok := r.users[username]
	if !ok {
		return User{}, errors.New("not found")
	}
	return user, nil
}

func (r *memoryRepo) IncrementFailedAttempts(_ context.Context, username string, lockedUntil *TimePtr) error {
	user := r.users[username]
	user.FailedAttempts++
	if lockedUntil != nil {
		t := time.Unix(0, lockedUntil.TimeUnixNano).UTC()
		user.LockedUntil = &t
	}
	r.users[username] = user
	return nil
}

func (r *memoryRepo) ResetFailedAttempts(_ context.Context, username string) error {
	user := r.users[username]
	user.FailedAttempts = 0
	user.LockedUntil = nil
	r.users[username] = user
	return nil
}

func (r *memoryRepo) SetLastLogin(_ context.Context, username string, at TimePtr) error {
	user := r.users[username]
	t := time.Unix(0, at.TimeUnixNano).UTC()
	user.LastLoginAt = &t
	r.users[username] = user
	return nil
}

func (r *memoryRepo) SetTOTP(_ context.Context, username, secret string, enabled bool) error {
	user := r.users[username]
	user.TOTPSecret = secret
	user.MFAEnabled = enabled
	r.users[username] = user
	return nil
}

func testService(repo *memoryRepo) *Service {
	return NewService(repo, Options{
		SessionTimeout:    time.Minute,
		MaxFailedAttempts: 3,
		LockoutDuration:   10 * time.Minute,
		BCryptCost:        bcrypt.MinCost,
		Clock:             fakeClock{now: time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)},
	})
}

func TestRegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepo()
	service := testService(repo)

	if _, err := service.Register(ctx, "Alice_1", "strong-password"); err != nil {
		t.Fatalf("register: %v", err)
	}
	result, err := service.Login(ctx, "alice_1", "strong-password", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result.Session.ID == "" {
		t.Fatal("expected session id")
	}
	if result.User.LastLoginAt == nil {
		t.Fatal("expected last login to be set")
	}
}

func TestDuplicateRegistration(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepo()
	service := testService(repo)

	if _, err := service.Register(ctx, "alice", "strong-password"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := service.Register(ctx, "alice", "strong-password"); !errors.Is(err, ErrDuplicateUser) {
		t.Fatalf("expected duplicate user, got %v", err)
	}
}

func TestLockoutAfterFailedAttempts(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepo()
	service := testService(repo)

	if _, err := service.Register(ctx, "alice", "strong-password"); err != nil {
		t.Fatalf("register: %v", err)
	}
	for i := 0; i < 3; i++ {
		_, _ = service.Login(ctx, "alice", "wrong-password", "")
	}
	_, err := service.Login(ctx, "alice", "strong-password", "")
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("expected account lockout, got %v", err)
	}
}

func TestSessionExpires(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepo()
	clock := &mutableClock{now: time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)}
	service := NewService(repo, Options{
		SessionTimeout:    time.Minute,
		MaxFailedAttempts: 3,
		LockoutDuration:   10 * time.Minute,
		BCryptCost:        bcrypt.MinCost,
		Clock:             clock,
	})
	if _, err := service.Register(ctx, "alice", "strong-password"); err != nil {
		t.Fatalf("register: %v", err)
	}
	result, err := service.Login(ctx, "alice", "strong-password", "")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	clock.now = clock.now.Add(2 * time.Minute)
	if _, err := service.Current(result.Session.ID); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("expected session expired, got %v", err)
	}
}

type mutableClock struct{ now time.Time }

func (m *mutableClock) Now() time.Time { return m.now }
