package main

import (
	"context"
	"database/sql"
	"testing"

	"vortex/test/testutil"

	_ "github.com/lib/pq"
)

// setupTestPG 启动 PostgreSQL 容器并建立连接，注册自动清理
func setupTestPG(t *testing.T) (*sql.DB, *testutil.PostgresContainer) {
	t.Helper()

	ctx := context.Background()
	pgContainer, err := testutil.NewPostgresContainer(ctx)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	db, err := sql.Open("postgres", pgContainer.ConnectionString)
	if err != nil {
		t.Fatalf("failed to open database connection: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		pgContainer.Cleanup(t)
	})

	return db, pgContainer
}
