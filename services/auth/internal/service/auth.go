// Package service holds the Auth use cases.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
)

type Auth struct {
	repo   domain.Repository
	hasher domain.PasswordHasher
	tokens domain.TokenIssuer
	log    *slog.Logger

	// dummyHash is compared against when the login is unknown, so "no such
	// user" costs as much time as "wrong password" and cannot be told apart.
	dummyHash string

	refresh    domain.RefreshStore // nil: no refresh tokens
	refreshTTL time.Duration
}

// Option configures optional Auth features.
type Option func(*Auth)

// WithRefresh enables refresh tokens (long sessions for API clients), kept
// in store and valid for ttl.
func WithRefresh(store domain.RefreshStore, ttl time.Duration) Option {
	return func(a *Auth) { a.refresh, a.refreshTTL = store, ttl }
}

func New(repo domain.Repository, hasher domain.PasswordHasher, tokens domain.TokenIssuer, log *slog.Logger, opts ...Option) (*Auth, error) {
	if log == nil {
		log = slog.Default()
	}
	dummy, err := hasher.Hash("dummy-password-for-timing-equalisation")
	if err != nil {
		return nil, fmt.Errorf("prepare dummy hash: %w", err)
	}
	a := &Auth{repo: repo, hasher: hasher, tokens: tokens, log: log, dummyHash: dummy}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// Register creates an account. The password is stored only as a bcrypt hash.
func (a *Auth) Register(ctx context.Context, username, email, password string) (domain.User, error) {
	username, email, err := domain.NormalizeRegistration(username, email, password)
	if err != nil {
		return domain.User{}, err
	}
	hash, err := a.hasher.Hash(password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	u, err := a.repo.Create(ctx, domain.User{Username: username, Email: email, PasswordHash: hash})
	if err != nil {
		return domain.User{}, err // ErrConflict passes through
	}
	a.log.Info("user registered", "user_id", u.ID)
	return u, nil
}

type LoginResult struct {
	Token   string
	Expires time.Time
	User    domain.User
	// A refresh token, when one was asked for (see IssueRefresh, Refresh).
	RefreshToken   string
	RefreshExpires time.Time
}

// Login verifies credentials and returns a signed token whose subject is the
// user's id. Wrong password and unknown user are indistinguishable.
func (a *Auth) Login(ctx context.Context, login, password string) (LoginResult, error) {
	login = strings.TrimSpace(login)
	if login == "" || password == "" {
		return LoginResult{}, fmt.Errorf("%w: login and password are required", domain.ErrInvalidInput)
	}

	u, err := a.repo.FindByLogin(ctx, login)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		_ = a.hasher.Compare(a.dummyHash, password) // burn the same time as a real check
		a.log.Info("login failed", "reason", "unknown_login")
		return LoginResult{}, domain.ErrInvalidCredentials
	case err != nil:
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}

	if len(password) > domain.MaxPasswordLen || a.hasher.Compare(u.PasswordHash, password) != nil {
		a.log.Info("login failed", "reason", "bad_password", "user_id", u.ID)
		return LoginResult{}, domain.ErrInvalidCredentials
	}

	tok, exp, err := a.tokens.Issue(u.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", err)
	}
	a.log.Info("login succeeded", "user_id", u.ID)
	return LoginResult{Token: tok, Expires: exp, User: u}, nil
}

// Refresh tokens: 32 random bytes (base64url), stored only as a SHA-256.
// They're high-entropy, so a fast hash is enough (unlike passwords).

const refreshBytes = 32

func newRefreshToken() (token string, hash []byte, err error) {
	b := make([]byte, refreshBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashRefresh(token), nil
}

func hashRefresh(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newFamilyID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // UUID v4
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// IssueRefresh starts a refresh-token family for a user who just signed in.
func (a *Auth) IssueRefresh(ctx context.Context, userID string) (string, time.Time, error) {
	if a.refresh == nil {
		return "", time.Time{}, errors.New("refresh tokens are not enabled")
	}
	family, err := newFamilyID()
	if err != nil {
		return "", time.Time{}, err
	}
	token, hash, err := newRefreshToken()
	if err != nil {
		return "", time.Time{}, err
	}
	exp := time.Now().Add(a.refreshTTL)
	if err := a.refresh.SaveRefresh(ctx, userID, family, hash, exp); err != nil {
		return "", time.Time{}, fmt.Errorf("save refresh token: %w", err)
	}
	return token, exp, nil
}

// Refresh exchanges a refresh token for a new access token and the next
// refresh token (the presented one is consumed). A replayed token revokes
// its family: whoever holds the newer one is signed out too, and signs in
// again with their password.
func (a *Auth) Refresh(ctx context.Context, token string) (LoginResult, error) {
	if a.refresh == nil || token == "" {
		return LoginResult{}, domain.ErrInvalidToken
	}
	next, nextHash, err := newRefreshToken()
	if err != nil {
		return LoginResult{}, err
	}
	exp := time.Now().Add(a.refreshTTL)
	userID, err := a.refresh.RotateRefresh(ctx, hashRefresh(token), nextHash, exp)
	switch {
	case errors.Is(err, domain.ErrTokenReused):
		a.log.Warn("refresh token reused: family revoked")
		return LoginResult{}, domain.ErrInvalidToken
	case err != nil:
		return LoginResult{}, err
	}
	u, err := a.repo.FindByID(ctx, userID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("find user: %w", err)
	}
	tok, texp, err := a.tokens.Issue(u.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("issue token: %w", err)
	}
	return LoginResult{Token: tok, Expires: texp, User: u, RefreshToken: next, RefreshExpires: exp}, nil
}

// RevokeRefresh signs a refresh-token family out (logout). Unknown tokens
// are ignored.
func (a *Auth) RevokeRefresh(ctx context.Context, token string) error {
	if a.refresh == nil || token == "" {
		return nil
	}
	return a.refresh.RevokeRefresh(ctx, hashRefresh(token))
}
