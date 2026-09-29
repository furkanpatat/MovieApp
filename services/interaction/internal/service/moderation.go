package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

// Moderation is what keeps user comments in check: reporting a comment and
// blocking a user. Reports are reviewed by hand (see init.sql).
type Moderation struct{ store domain.ModerationStore }

func NewModeration(store domain.ModerationStore) *Moderation { return &Moderation{store: store} }

// ReportComment: commentID is a comment's id as the API returns it.
func (m *Moderation) ReportComment(ctx context.Context, reporterID, commentID string) error {
	if _, err := uuid.Parse(commentID); err != nil {
		return domain.ErrInvalidInput
	}
	return m.store.ReportComment(ctx, commentID, reporterID)
}

func (m *Moderation) Block(ctx context.Context, blockerID, blockedID string) error {
	if err := checkTarget(blockerID, blockedID); err != nil {
		return err
	}
	return m.store.BlockUser(ctx, blockerID, blockedID)
}

func (m *Moderation) Unblock(ctx context.Context, blockerID, blockedID string) error {
	if err := checkTarget(blockerID, blockedID); err != nil {
		return err
	}
	return m.store.UnblockUser(ctx, blockerID, blockedID)
}

func (m *Moderation) Blocked(ctx context.Context, blockerID string) ([]string, error) {
	return m.store.BlockedUsers(ctx, blockerID)
}

// checkTarget: someone else, and a plausible user id.
func checkTarget(self, other string) error {
	other = strings.TrimSpace(other)
	if other == "" || len(other) > domain.MaxUserIDLen || other == self {
		return domain.ErrInvalidInput
	}
	return nil
}
