package domain

import (
	"context"
	"time"
)

// Person is an actor or other film professional, with the movies they are
// credited on (cast and crew, most popular first).
type Person struct {
	ID                 int      `json:"id"`
	Name               string   `json:"name"`
	Biography          string   `json:"biography,omitempty"`
	ProfilePath        string   `json:"profile_path,omitempty"`
	Birthday           string   `json:"birthday,omitempty"`
	Deathday           string   `json:"deathday,omitempty"`
	PlaceOfBirth       string   `json:"place_of_birth,omitempty"`
	KnownForDepartment string   `json:"known_for_department,omitempty"`
	Credits            []Credit `json:"credits"`
}

// Credit is one movie in a person's filmography. The movie's fields are
// inlined in JSON, so a credit renders anywhere a movie card does.
type Credit struct {
	Movie
	Character string `json:"character,omitempty"` // cast credits
	Job       string `json:"job,omitempty"`       // crew credits (Director, Writer...)
}

// PersonStore is the L2 persistent cache for people.
type PersonStore interface {
	UpsertPerson(ctx context.Context, p Person) error
	// GetPerson returns ErrNotFound when the person is not stored.
	GetPerson(ctx context.Context, id int) (Person, time.Time, error)
}
