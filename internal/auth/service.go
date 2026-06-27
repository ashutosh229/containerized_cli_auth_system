package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrAccountLocked      = errors.New("account is temporarily locked")
	ErrDuplicateUser      = errors.New("username already exists")
	ErrMFARequired        = errors.New("totp code is required")
	ErrInvalidTOTP        = errors.New("invalid totp code")
	ErrSessionExpired     = errors.New("session expired")
	ErrNotLoggedIn        = errors.New("not logged in")
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type Options struct {
	SessionTimeout    time.Duration
	MaxFailedAttempts int
	LockoutDuration   time.Duration
	TOTPIssuer        string
	BCryptCost        int
	Clock             Clock
}

type Service struct {
	users    UserRepository
	options  Options
	sessions map[string]Session
	mu       sync.Mutex
}

type LoginResult struct {
	Session Session
	User    User
}

func NewService(users UserRepository, options Options) *Service {
	if options.SessionTimeout <= 0 {
		options.SessionTimeout = 30 * time.Minute
	}
	if options.MaxFailedAttempts <= 0 {
		options.MaxFailedAttempts = 5
	}
	if options.LockoutDuration <= 0 {
		options.LockoutDuration = 15 * time.Minute
	}
	if options.BCryptCost == 0 {
		options.BCryptCost = bcrypt.DefaultCost
	}
	if options.Clock == nil {
		options.Clock = RealClock{}
	}
	if options.TOTPIssuer == "" {
		options.TOTPIssuer = "Containerized CLI Auth"
	}
	return &Service{users: users, options: options, sessions: map[string]Session{}}
}

func (s *Service) Register(ctx context.Context, username, password string) (User, error) {
	username = normalizeUsername(username)
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	if err := validatePassword(password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.options.BCryptCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.users.CreateUser(ctx, username, hash)
	if err != nil {
		if errors.Is(err, ErrDuplicateUser) {
			return User{}, ErrDuplicateUser
		}
		return User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, username, password, totpCode string) (LoginResult, error) {
	username = normalizeUsername(username)
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	now := s.options.Clock.Now()
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return LoginResult{}, fmt.Errorf("%w until %s", ErrAccountLocked, user.LockedUntil.Format(time.RFC3339))
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		if err := s.recordFailedLogin(ctx, user, now); err != nil {
			return LoginResult{}, err
		}
		return LoginResult{}, ErrInvalidCredentials
	}
	if user.MFAEnabled {
		if strings.TrimSpace(totpCode) == "" {
			return LoginResult{}, ErrMFARequired
		}
		if !totp.Validate(strings.TrimSpace(totpCode), user.TOTPSecret) {
			if err := s.recordFailedLogin(ctx, user, now); err != nil {
				return LoginResult{}, err
			}
			return LoginResult{}, ErrInvalidTOTP
		}
	}
	if err := s.users.ResetFailedAttempts(ctx, username); err != nil {
		return LoginResult{}, err
	}
	nowPtr := TimePtr{TimeUnixNano: now.UnixNano()}
	if err := s.users.SetLastLogin(ctx, username, nowPtr); err != nil {
		return LoginResult{}, err
	}
	user.LastLoginAt = &now
	session, err := s.newSession(user, now)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Session: session, User: user}, nil
}

func (s *Service) Current(sessionID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, ErrNotLoggedIn
	}
	if !session.ExpiresAt.After(s.options.Clock.Now()) {
		delete(s.sessions, sessionID)
		return Session{}, ErrSessionExpired
	}
	return session, nil
}

func (s *Service) RefreshSession(ctx context.Context, sessionID string) (Session, error) {
	session, err := s.Current(sessionID)
	if err != nil {
		return Session{}, err
	}
	user, err := s.users.FindByUsername(ctx, session.User.Username)
	if err != nil {
		return Session{}, err
	}
	session.User = user

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = session
	return session, nil
}

func (s *Service) Logout(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

func (s *Service) BeginEnableTOTP(username string) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      s.options.TOTPIssuer,
		AccountName: username,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) ConfirmEnableTOTP(ctx context.Context, username, secret, code string) error {
	if !totp.Validate(strings.TrimSpace(code), secret) {
		return ErrInvalidTOTP
	}
	return s.users.SetTOTP(ctx, username, secret, true)
}

func (s *Service) DisableTOTP(ctx context.Context, username, password, code string) error {
	user, err := s.users.FindByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return ErrInvalidCredentials
	}
	if user.MFAEnabled && !totp.Validate(strings.TrimSpace(code), user.TOTPSecret) {
		return ErrInvalidTOTP
	}
	return s.users.SetTOTP(ctx, username, "", false)
}

func (s *Service) recordFailedLogin(ctx context.Context, user User, now time.Time) error {
	attempts := user.FailedAttempts + 1
	var lockout *TimePtr
	if attempts >= s.options.MaxFailedAttempts {
		until := now.Add(s.options.LockoutDuration)
		lockout = &TimePtr{TimeUnixNano: until.UnixNano()}
	}
	return s.users.IncrementFailedAttempts(ctx, user.Username, lockout)
}

func (s *Service) newSession(user User, now time.Time) (Session, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return Session{}, err
	}
	session := Session{
		ID:        hex.EncodeToString(tokenBytes),
		User:      user,
		ExpiresAt: now.Add(s.options.SessionTimeout),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return session, nil
}

func normalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return errors.New("username must be 3-32 characters")
	}
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return errors.New("username may contain only letters, numbers, underscores, and hyphens")
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}
