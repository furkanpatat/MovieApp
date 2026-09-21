// Package service holds the Auth use cases.
package service

import (
	"context"
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
}

func New(repo domain.Repository, hasher domain.PasswordHasher, tokens domain.TokenIssuer, log *slog.Logger) (*Auth, error) {
	if log == nil {
		log = slog.Default()
	}
	dummy, err := hasher.Hash("dummy-password-for-timing-equalisation")
	if err != nil {
		return nil, fmt.Errorf("prepare dummy hash: %w", err)
	}
	return &Auth{repo: repo, hasher: hasher, tokens: tokens, log: log, dummyHash: dummy}, nil
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
