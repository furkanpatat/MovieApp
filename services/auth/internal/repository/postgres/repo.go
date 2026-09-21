// Package postgres stores users.
package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
)

const (
	defaultSchema       = "auth"
	uniqueViolationCode = "23505"
)

type Repo struct {
	pool   *pgxpool.Pool
	schema string
}

var _ domain.Repository = (*Repo)(nil)

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool, schema: defaultSchema} }

// NewInSchema targets another schema with the same tables (isolated tests).
// schema must be a trusted identifier.
func NewInSchema(pool *pgxpool.Pool, schema string) *Repo { return &Repo{pool: pool, schema: schema} }

func (r *Repo) q(sql string) string { return strings.ReplaceAll(sql, defaultSchema+".", r.schema+".") }

func (r *Repo) Create(ctx context.Context, u domain.User) (domain.User, error) {
	err := r.pool.QueryRow(ctx,
		r.q(`INSERT INTO auth.users (username, email, password_hash) VALUES ($1, $2, $3)
		     RETURNING id::text, created_at`),
		u.Username, u.Email, u.PasswordHash).Scan(&u.ID, &u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
		return domain.User{}, domain.ErrConflict
	}
	return u, err
}

func (r *Repo) FindByLogin(ctx context.Context, login string) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		r.q(`SELECT id::text, username, email, password_hash, created_at FROM auth.users
		      WHERE lower(username) = lower($1) OR lower(email) = lower($1)`), login).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}
