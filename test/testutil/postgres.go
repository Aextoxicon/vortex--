package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultDatabase = "vortex_test"
	DefaultUsername = "test"
	DefaultPassword = "test"
)

type PostgresContainer struct {
	*postgres.PostgresContainer
	ConnectionString string
}

func NewPostgresContainer(ctx context.Context) (*PostgresContainer, error) {
	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase(DefaultDatabase),
		postgres.WithUsername(DefaultUsername),
		postgres.WithPassword(DefaultPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	return &PostgresContainer{
		PostgresContainer: pgContainer,
		ConnectionString:  connStr,
	}, nil
}

func (p *PostgresContainer) Cleanup(t interface {
	Logf(format string, v ...interface{})
}) {
	t.Logf("terminating postgres container")
	if err := p.Terminate(context.Background()); err != nil {
		t.Logf("failed to terminate postgres container: %v", err)
	}
}
