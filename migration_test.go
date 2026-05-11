package main

import (
	"context"
	"database/sql"
	"testing"

	"vortex/test/testutil"

	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) (*sql.DB, *testutil.PostgresContainer) {
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

func TestRunMigrations(t *testing.T) {
	db, _ := setupTestDB(t)

	err := RunMigrations(db)
	if err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	tables := []string{
		"users",
		"groups",
		"group_members",
		"friend_requests",
		"conversation_participants",
		"id_generator_state",
		"messages",
		"jwt_blacklist",
	}

	for _, table := range tables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' 
				AND table_name = $1
			)`, table).Scan(&exists)

		if err != nil {
			t.Errorf("failed to check table %s: %v", table, err)
			continue
		}

		if !exists {
			t.Errorf("table %s was not created", table)
		}
	}
}

func TestMigrations_CreateIndexes(t *testing.T) {
	db, _ := setupTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	indexes := []struct {
		table string
		index string
	}{
		{"users", "idx_users_username"},
		{"users", "idx_users_public_id"},
		{"groups", "idx_groups_owner_id"},
		{"group_members", "idx_group_members_uid"},
		{"friend_requests", "idx_friend_requests_from_user_id"},
		{"friend_requests", "idx_friend_requests_to_user_id"},
	}

	for _, idx := range indexes {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM pg_indexes 
				WHERE schemaname = 'public' 
				AND indexname = $1
			)`, idx.index).Scan(&exists)

		if err != nil {
			t.Errorf("failed to check index %s: %v", idx.index, err)
			continue
		}

		if !exists {
			t.Errorf("index %s on table %s was not created", idx.index, idx.table)
		}
	}
}

func TestMigrations_Idempotency(t *testing.T) {
	db, _ := setupTestDB(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("first migration run failed: %v", err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("second migration run failed: %v", err)
	}
}
