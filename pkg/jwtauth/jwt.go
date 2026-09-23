// Package jwtauth issues and verifies the platform's access tokens (HS256).
// The Auth service issues them and the Gateway verifies them; sharing this code
// guarantees both sides agree on the format.
package jwtauth

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// DefaultIssuer is the `iss` claim unless configured otherwise.
const DefaultIssuer = "movieapp-auth"

// Browser sessions carry the token in an HttpOnly cookie instead of JS-readable
// storage: the Auth service sets it at login, the Gateway reads it. Scoped to
// the API so page requests never carry it.
const (
	CookieName = "movieapp_session"
	CookiePath = "/api/"
)

var ErrInvalidToken = errors.New("invalid token")

// validUserID keeps identities safe to forward as an HTTP header and to store.
var validUserID = regexp.MustCompile(`^[A-Za-z0-9._@:-]{1,64}$`)

func ValidUserID(id string) bool { return validUserID.MatchString(id) }

// Manager signs (HS256) and verifies tokens. The user id lives in `sub`.
type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
	parser *jwt.Parser
}

func NewManager(secret, issuer string, ttl time.Duration) *Manager {
	return newManager(secret, issuer, ttl, time.Now)
}

func newManager(secret, issuer string, ttl time.Duration, now func() time.Time) *Manager {
	return &Manager{
		secret: []byte(secret), issuer: issuer, ttl: ttl, now: now,
		parser: jwt.NewParser(
			// Pin the algorithm: rejects "none" and any RS/ES confusion attack.
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(issuer),
			jwt.WithExpirationRequired(),
			jwt.WithIssuedAt(),
			jwt.WithLeeway(5*time.Second),
			jwt.WithTimeFunc(now),
		),
	}
}

func (m *Manager) TTL() time.Duration { return m.ttl }

// Issue returns a signed token for userID.
func (m *Manager) Issue(userID string) (string, time.Time, error) {
	if !ValidUserID(userID) {
		return "", time.Time{}, fmt.Errorf("invalid user id")
	}
	now := m.now()
	exp := now.Add(m.ttl)
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    m.issuer,
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	s, err := tok.SignedString(m.secret)
	return s, exp, err
}

// Verify checks signature, algorithm, issuer and expiry, and returns the user id.
func (m *Manager) Verify(token string) (string, error) {
	var claims jwt.RegisteredClaims
	tok, err := m.parser.ParseWithClaims(token, &claims, func(*jwt.Token) (any, error) { return m.secret, nil })
	if err != nil || !tok.Valid {
		return "", ErrInvalidToken
	}
	if !ValidUserID(claims.Subject) {
		return "", ErrInvalidToken
	}
	return claims.Subject, nil
}
