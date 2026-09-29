package service

import (
	"context"
	"fmt"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// Account is account deletion's part here: everything a user wrote goes.
type Account struct {
	repo domain.Repository
	rm   domain.ReadModel
}

func NewAccount(repo domain.Repository, rm domain.ReadModel) *Account {
	return &Account{repo: repo, rm: rm}
}

// PurgeUser deletes the user's ratings and comments, then drops the read
// models of the titles they touched: the next read rebuilds each from
// Postgres. Safe to repeat.
func (a *Account) PurgeUser(ctx context.Context, userID string) error {
	touched, err := a.repo.PurgeUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("purge user: %w", err)
	}
	for _, s := range touched {
		if err := a.rm.Delete(ctx, s.Title); err != nil {
			return fmt.Errorf("drop read model: %w", err)
		}
	}
	return nil
}
