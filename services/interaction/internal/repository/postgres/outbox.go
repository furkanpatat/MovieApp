package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanpatat/movieapp/services/interaction/internal/domain"
)

var (
	_ domain.Outbox      = (*Repo)(nil)
	_ domain.OutboxStore = (*Repo)(nil)
)

// Enqueue records the event. Idempotent on the event id.
func (r *Repo) Enqueue(ctx context.Context, m domain.OutboxMessage) error {
	_, err := r.pool.Exec(ctx,
		r.q(`INSERT INTO interaction.outbox_events (id, event_type, payload) VALUES ($1, $2, $3)
		 ON CONFLICT (id) DO NOTHING`), m.ID, m.Type, m.Payload)
	return err
}

// PublishPending claims a batch with FOR UPDATE SKIP LOCKED, so several relay
// instances can run side by side without publishing the same row twice at the
// same time. Rows stay locked until commit; if the process dies mid-batch the
// locks vanish with the connection and another relay retries (at-least-once).
func (r *Repo) PublishPending(ctx context.Context, limit int, publish func(context.Context, domain.OutboxMessage) error) (int, error) {
	var published int
	var pubErr error

	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			r.q(`SELECT id::text, event_type, payload FROM interaction.outbox_events
			  WHERE status = 'pending' ORDER BY created_at, id LIMIT $1 FOR UPDATE SKIP LOCKED`), limit)
		if err != nil {
			return err
		}
		var batch []domain.OutboxMessage
		for rows.Next() {
			var m domain.OutboxMessage
			if err := rows.Scan(&m.ID, &m.Type, &m.Payload); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		var done []string
		for _, m := range batch {
			if err := publish(ctx, m); err != nil {
				pubErr = err
				// Record why, for observability; keep the row pending.
				if _, err := tx.Exec(ctx,
					r.q(`UPDATE interaction.outbox_events SET attempts = attempts + 1, last_error = $2 WHERE id = $1`),
					m.ID, truncate(err.Error(), 500)); err != nil {
					return err
				}
				break // broker trouble: don't hammer it, keep order
			}
			done = append(done, m.ID)
		}
		published = len(done)
		if len(done) == 0 {
			return nil
		}
		_, err = tx.Exec(ctx,
			r.q(`UPDATE interaction.outbox_events SET status = 'published', published_at = now(), last_error = NULL
			  WHERE id = ANY($1::uuid[])`), done)
		return err
	})
	if err != nil {
		return 0, err // the tx rolled back: nothing is marked, the batch will be retried
	}
	return published, pubErr
}

func (r *Repo) DeleteExpired(ctx context.Context, retention time.Duration, limit int) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		r.q(`DELETE FROM interaction.outbox_events WHERE id IN (
		   SELECT id FROM interaction.outbox_events
		    WHERE status = 'published' AND published_at < now() - make_interval(secs => $1)
		    LIMIT $2)`), retention.Seconds(), limit)
	return tag.RowsAffected(), err
}

func (r *Repo) PendingCount(ctx context.Context) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, r.q(`SELECT count(*) FROM interaction.outbox_events WHERE status = 'pending'`)).Scan(&n)
	return n, err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
