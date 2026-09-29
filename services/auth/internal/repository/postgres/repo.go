// Package postgres stores users.
package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

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

func (r *Repo) FindByID(ctx context.Context, id string) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		r.q(`SELECT id::text, username, email, password_hash, created_at FROM auth.users WHERE id::text = $1`), id).
		Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

var _ domain.RefreshStore = (*Repo)(nil)

func (r *Repo) SaveRefresh(ctx context.Context, userID, family string, hash []byte, expires time.Time) error {
	// Housekeeping while we're here: the user's tokens long past expiry.
	if _, err := r.pool.Exec(ctx,
		r.q(`DELETE FROM auth.refresh_tokens WHERE user_id = $1 AND expires_at < now() - interval '1 day'`), userID); err != nil {
		return err
	}
	_, err := r.pool.Exec(ctx,
		r.q(`INSERT INTO auth.refresh_tokens (user_id, family_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`),
		userID, family, hash, expires)
	return err
}

// RotateRefresh locks the presented token's row, so two requests racing
// with the same token are serialized: one rotates, the other sees it used.
func (r *Repo) RotateRefresh(ctx context.Context, oldHash, newHash []byte, expires time.Time) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		id, userID, family string
		expiresAt          time.Time
		usedAt, revokedAt  *time.Time
	)
	err = tx.QueryRow(ctx,
		r.q(`SELECT id::text, user_id::text, family_id::text, expires_at, used_at, revoked_at
		       FROM auth.refresh_tokens WHERE token_hash = $1 FOR UPDATE`), oldHash).
		Scan(&id, &userID, &family, &expiresAt, &usedAt, &revokedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", domain.ErrInvalidToken
	case err != nil:
		return "", err
	case revokedAt != nil:
		return "", domain.ErrInvalidToken
	case usedAt != nil:
		// A consumed token replayed: revoke the family, keep that revocation.
		if _, err := tx.Exec(ctx,
			r.q(`UPDATE auth.refresh_tokens SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`), family); err != nil {
			return "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
		return "", domain.ErrTokenReused
	case !expiresAt.After(time.Now()):
		return "", domain.ErrInvalidToken
	}

	if _, err := tx.Exec(ctx, r.q(`UPDATE auth.refresh_tokens SET used_at = now() WHERE id = $1`), id); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		r.q(`INSERT INTO auth.refresh_tokens (user_id, family_id, token_hash, expires_at) VALUES ($1, $2, $3, $4)`),
		userID, family, newHash, expires); err != nil {
		return "", err
	}
	return userID, tx.Commit(ctx)
}

func (r *Repo) RevokeRefresh(ctx context.Context, hash []byte) error {
	_, err := r.pool.Exec(ctx,
		r.q(`UPDATE auth.refresh_tokens SET revoked_at = now()
		      WHERE family_id = (SELECT family_id FROM auth.refresh_tokens WHERE token_hash = $1) AND revoked_at IS NULL`), hash)
	return err
}

func (r *Repo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, r.q(`DELETE FROM auth.users WHERE id::text = $1`), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
