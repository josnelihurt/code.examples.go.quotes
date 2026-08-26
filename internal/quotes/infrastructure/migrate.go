package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers the pgx5:// scheme
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/josnelihurt/code.examples.go.quotes/migrations"
)

// Migrate applies every embedded migration up to the latest version — the Go
// analogue of the .NET host's Database.MigrateAsync() at boot. It is
// idempotent (a second run is a no-op) and serialized across replicas by
// golang-migrate's schema_migrations advisory lock, so competing boots wait
// their turn instead of racing the DDL.
func Migrate(ctx context.Context, databaseURL string) error {
	dsn, err := migrateDSN(databaseURL)
	if err != nil {
		return err
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("opening the embedded migrations: %w", err)
	}

	migrator, err := migrate.NewWithSourceInstance("iofs", source, dsn)
	if err != nil {
		return fmt.Errorf("creating the migrator: %w", err)
	}

	// golang-migrate v4 has no context-aware Up; the wall clock keeps a boot
	// shutdown from hanging on a frozen socket. The goroutine owns the
	// migrator exclusively — including its Close — so the two can never race.
	done := make(chan error, 1)
	go func() {
		defer func() {
			_, _ = migrator.Close() // (error, isLocked) — nothing either side can act on here
		}()

		err := migrator.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			err = nil
		} else if err != nil {
			err = fmt.Errorf("applying the migrations: %w", err)
		}
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("the migrations were aborted: %w", ctx.Err())
	}
}

// migrateDSN rewrites a postgres:// URL into the pgx5:// scheme the
// golang-migrate pgx/v5 driver registers under (the driver itself connects
// with plain postgres:// semantics underneath).
func migrateDSN(databaseURL string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parsing the database url: %w", err)
	}

	switch parsed.Scheme {
	case "postgres", "postgresql":
		parsed.Scheme = "pgx5"
	case "pgx5":
	default:
		return "", fmt.Errorf("unsupported database url scheme %q", parsed.Scheme)
	}

	return parsed.String(), nil
}
