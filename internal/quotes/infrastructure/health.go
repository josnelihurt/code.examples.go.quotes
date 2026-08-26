package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PingBudget is the wall-clock budget every catalog database round-trip must
// answer within (parity with the .NET quotesdb-roundtrip check).
const PingBudget = 5 * time.Second

// Ping proves the database actually answers a SELECT 1 round-trip, not merely
// that a warm pool holds an idle connection — a paused database would pass
// that weaker check and still be down. The budget is a select guard rather
// than a deadline handed to the query: a socket frozen mid-read can ignore
// cooperative cancellation, and readiness must answer regardless.
func Ping(ctx context.Context, pool *pgxpool.Pool, budget time.Duration) error {
	done := make(chan error, 1)
	go func() {
		_, err := pool.Exec(ctx, "SELECT 1")
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("the catalog database round-trip failed: %w", err)
		}
		return nil
	case <-time.After(budget):
		return fmt.Errorf("the catalog database round-trip exceeded the %s budget", budget)
	}
}
