package testsupport

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/furkanpatat/movieapp/services/auth/internal/domain"
)

// MemRefresh is an in-memory domain.RefreshStore with the Postgres
// repository's semantics (rotation, reuse revokes the family).
type MemRefresh struct {
	mu   sync.Mutex
	Rows map[string]*RefreshRow // by hex(hash)
}

type RefreshRow struct {
	User, Family  string
	Expires       time.Time
	Used, Revoked bool
}

var _ domain.RefreshStore = (*MemRefresh)(nil)

func NewMemRefresh() *MemRefresh { return &MemRefresh{Rows: map[string]*RefreshRow{}} }

func (m *MemRefresh) SaveRefresh(_ context.Context, userID, family string, hash []byte, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Rows[hex.EncodeToString(hash)] = &RefreshRow{User: userID, Family: family, Expires: expires}
	return nil
}

func (m *MemRefresh) RotateRefresh(_ context.Context, oldHash, newHash []byte, expires time.Time) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.Rows[hex.EncodeToString(oldHash)]
	switch {
	case !ok || row.Revoked:
		return "", domain.ErrInvalidToken
	case row.Used:
		for _, r := range m.Rows {
			if r.Family == row.Family {
				r.Revoked = true
			}
		}
		return "", domain.ErrTokenReused
	case !row.Expires.After(time.Now()):
		return "", domain.ErrInvalidToken
	}
	row.Used = true
	m.Rows[hex.EncodeToString(newHash)] = &RefreshRow{User: row.User, Family: row.Family, Expires: expires}
	return row.User, nil
}

func (m *MemRefresh) RevokeRefresh(_ context.Context, hash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if row, ok := m.Rows[hex.EncodeToString(hash)]; ok {
		for _, r := range m.Rows {
			if r.Family == row.Family {
				r.Revoked = true
			}
		}
	}
	return nil
}
