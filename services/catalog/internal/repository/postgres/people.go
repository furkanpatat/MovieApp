package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanpatat/movieapp/services/catalog/internal/domain"
)

// UpsertPerson inserts or refreshes a person and their credits.
func (r *Repository) UpsertPerson(ctx context.Context, p domain.Person) error {
	credits := p.Credits
	if credits == nil {
		credits = []domain.Credit{}
	}
	creditsJSON, err := json.Marshal(credits)
	if err != nil {
		return fmt.Errorf("encode credits: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO people (
			id, name, biography, profile_path, birthday, deathday,
			place_of_birth, known_for_department, credits, fetched_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			biography = EXCLUDED.biography,
			profile_path = EXCLUDED.profile_path,
			birthday = EXCLUDED.birthday,
			deathday = EXCLUDED.deathday,
			place_of_birth = EXCLUDED.place_of_birth,
			known_for_department = EXCLUDED.known_for_department,
			credits = EXCLUDED.credits,
			fetched_at = NOW()`,
		p.ID, p.Name, p.Biography, nullIfEmpty(p.ProfilePath), nullIfEmpty(p.Birthday), nullIfEmpty(p.Deathday),
		nullIfEmpty(p.PlaceOfBirth), nullIfEmpty(p.KnownForDepartment), string(creditsJSON),
	)
	return err
}

// GetPerson returns a stored person and when it was fetched from TMDB.
func (r *Repository) GetPerson(ctx context.Context, id int) (domain.Person, time.Time, error) {
	var p domain.Person
	var profile, birthday, deathday, place, dept *string
	var creditsJSON []byte
	var fetchedAt time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, biography, profile_path, birthday, deathday,
			place_of_birth, known_for_department, credits, fetched_at
		FROM people WHERE id = $1`, id).Scan(
		&p.ID, &p.Name, &p.Biography, &profile, &birthday, &deathday, &place, &dept, &creditsJSON, &fetchedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Person{}, time.Time{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Person{}, time.Time{}, err
	}
	p.ProfilePath, p.Birthday, p.Deathday = deref(profile), deref(birthday), deref(deathday)
	p.PlaceOfBirth, p.KnownForDepartment = deref(place), deref(dept)
	if err := json.Unmarshal(creditsJSON, &p.Credits); err != nil {
		return domain.Person{}, time.Time{}, fmt.Errorf("decode credits for person %d: %w", id, err)
	}
	return p, fetchedAt, nil
}
