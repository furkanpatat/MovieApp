// Package domain holds the Auth entities, rules and ports.
package domain

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrConflict           = errors.New("username or email already registered")
	ErrNotFound           = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken: a refresh token that is unknown, expired or revoked.
	ErrInvalidToken = errors.New("invalid or expired refresh token")
	// ErrTokenReused: a refresh token that was already exchanged came back.
	// Only a stolen copy does that, so its whole family is revoked.
	ErrTokenReused = errors.New("refresh token reused")
)

const (
	MinPasswordLen = 8
	// MaxPasswordLen is bcrypt's hard limit: it ignores everything past 72
	// bytes, so longer passwords are rejected instead of silently truncated.
	MaxPasswordLen = 72
	MaxEmailLen    = 254
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

type User struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, a...))
}

// NormalizeRegistration validates and canonicalises registration input
// (trimmed username, lower-cased email).
func NormalizeRegistration(username, email, password string) (string, string, error) {
	username = strings.TrimSpace(username)
	email = strings.ToLower(strings.TrimSpace(email))

	if !usernameRE.MatchString(username) {
		return "", "", invalid("username must be 3-32 characters: letters, digits, '.', '_' or '-'")
	}
	if len(email) > MaxEmailLen {
		return "", "", invalid("email is too long")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return "", "", invalid("email is not valid")
	}
	if len(password) < MinPasswordLen {
		return "", "", invalid("password must be at least %d characters", MinPasswordLen)
	}
	if len(password) > MaxPasswordLen {
		return "", "", invalid("password must be at most %d bytes", MaxPasswordLen)
	}
	return username, email, nil
}

// Repository persists users.
type Repository interface {
	// Create stores the user and returns it with id and created_at filled in.
	// It returns ErrConflict if the username or email is taken (any case).
	Create(ctx context.Context, u User) (User, error)
	// FindByLogin looks a user up by username or email, case-insensitively.
	// It returns ErrNotFound if there is none.
	FindByLogin(ctx context.Context, login string) (User, error)
	// FindByID looks a user up by id. It returns ErrNotFound if there is none.
	FindByID(ctx context.Context, id string) (User, error)
}

// RefreshStore keeps refresh tokens, only as hashes, grouped in families:
// one family per sign-in, each refresh replacing its token with a new one
// (rotation).
type RefreshStore interface {
	// SaveRefresh stores a new token of userID in family.
	SaveRefresh(ctx context.Context, userID, family string, hash []byte, expires time.Time) error
	// RotateRefresh consumes the token with oldHash and stores newHash in its
	// family, atomically, and returns the user's id. An unknown, expired or
	// revoked token is ErrInvalidToken; one already consumed revokes its
	// family and is ErrTokenReused.
	RotateRefresh(ctx context.Context, oldHash, newHash []byte, expires time.Time) (userID string, err error)
	// RevokeRefresh revokes the family of the token with hash (sign-out). An
	// unknown token is not an error.
	RevokeRefresh(ctx context.Context, hash []byte) error
}

// PasswordHasher hashes and checks passwords.
type PasswordHasher interface {
	Hash(password string) (string, error)
	// Compare returns nil only when password matches hash.
	Compare(hash, password string) error
}

// TokenIssuer mints access tokens (satisfied by jwtauth.Manager).
type TokenIssuer interface {
	Issue(userID string) (token string, expires time.Time, err error)
}
