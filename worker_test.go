package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"vortex/test/testutil"

	_ "github.com/lib/pq"
)

func setupTestWorker(t *testing.T) (*Worker, *sql.DB, *testutil.PostgresContainer) {
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

	if err := RunMigrations(db); err != nil {
		db.Close()
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		pgContainer.Cleanup(t)
	})

	store := NewStore(db, 0)
	msgStore := &MessageStore{Store: store}

	cfg := &Config{
		NodeID:                               1,
		WorkerTableCreateIntervalHours:       24,
		WorkerMaintenanceInitialDelayMinutes: 1,
		WorkerMaintenanceIntervalHours:       24,
		MessageRetentionDays:                 7,
	}

	svc := &Service{}
	worker := NewWorker(cfg, svc, msgStore)

	return worker, db, pgContainer
}

func TestMessageTableNameByDate(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		expected string
	}{
		{
			name:     "2026-01-01",
			date:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			expected: "messages_20260101",
		},
		{
			name:     "2026-12-31",
			date:     time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
			expected: "messages_20261231",
		},
		{
			name:     "2026-06-15",
			date:     time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
			expected: "messages_20260615",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MessageTableNameByDate(tt.date)
			if got != tt.expected {
				t.Errorf("MessageTableNameByDate(%v) = %s, want %s", tt.date, got, tt.expected)
			}
		})
	}
}

func TestCalculateNextMondayDelay(t *testing.T) {
	// Test that the function returns a positive duration
	delay := calculateNextMondayDelay()
	if delay <= 0 {
		t.Errorf("expected positive delay, got %v", delay)
	}

	// The delay should be less than 7 days
	maxDelay := 7 * 24 * time.Hour
	if delay >= maxDelay {
		t.Errorf("delay too large: %v, should be less than %v", delay, maxDelay)
	}
}

func TestCalculateDaysToSunday(t *testing.T) {
	tests := []struct {
		name     string
		date     time.Time
		expected int
	}{
		{
			name:     "Sunday",
			date:     time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), // Sunday
			expected: 0,
		},
		{
			name:     "Monday",
			date:     time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC), // Monday
			expected: 6,
		},
		{
			name:     "Saturday",
			date:     time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC), // Saturday
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dayOfWeek := int(tt.date.Weekday())
			daysToSunday := 0
			if dayOfWeek != 0 {
				daysToSunday = 7 - dayOfWeek
			}
			if daysToSunday != tt.expected {
				t.Errorf("daysToSunday for %v = %d, want %d", tt.date, daysToSunday, tt.expected)
			}
		})
	}
}

func TestWorker_CreateTablesFromTodayToSunday(t *testing.T) {
	worker, db, _ := setupTestWorker(t)
	ctx := context.Background()

	worker.createTablesFromTodayToSunday()

	now := time.Now().UTC()
	dayOfWeek := int(now.Weekday())
	daysToSunday := 0
	if dayOfWeek != 0 {
		daysToSunday = 7 - dayOfWeek
	}

	for offset := 0; offset <= daysToSunday; offset++ {
		date := now.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)

		var exists bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' 
				AND table_name = $1
			)`, tableName).Scan(&exists)

		if err != nil {
			t.Errorf("failed to check table %s: %v", tableName, err)
			continue
		}

		if !exists {
			t.Errorf("table %s was not created", tableName)
		}
	}
}

func TestWorker_CreateWeekTables(t *testing.T) {
	worker, db, _ := setupTestWorker(t)
	ctx := context.Background()

	worker.createWeekTables()

	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	nextMonday := now.AddDate(0, 0, 8-weekday)

	for offset := 0; offset < 7; offset++ {
		date := nextMonday.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)

		var exists bool
		err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' 
				AND table_name = $1
			)`, tableName).Scan(&exists)

		if err != nil {
			t.Errorf("failed to check table %s: %v", tableName, err)
			continue
		}

		if !exists {
			t.Errorf("table %s was not created", tableName)
		}
	}
}

func TestWorker_RunAnalyze(t *testing.T) {
	worker, db, _ := setupTestWorker(t)
	ctx := context.Background()

	tableName := MessageTableNameByDate(time.Now())
	err := worker.msgStore.EnsurePartition(tableName)
	if err != nil {
		t.Fatalf("failed to create message partition: %v", err)
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf(
		"INSERT INTO %s (msg_id, conv_id, from_uid, content, ts) VALUES ($1, $2, $3, $4, $5)",
		tableName),
		time.Now().UnixNano(), "conv_test", 123, "test message", time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("failed to insert test message: %v", err)
	}

	worker.runAnalyze()

	var relname string
	err = db.QueryRowContext(ctx, `
		SELECT relname FROM pg_stat_user_tables 
		WHERE relname = $1 AND last_analyze IS NOT NULL`, tableName).Scan(&relname)

	if err != nil {
		t.Errorf("ANALYZE was not run on table %s: %v", tableName, err)
	}
}

func TestWorker_DropExpiredPartitions(t *testing.T) {
	worker, db, _ := setupTestWorker(t)
	ctx := context.Background()

	expiredDate := time.Now().UTC().AddDate(0, 0, -10)
	expiredTableName := MessageTableNameByDate(expiredDate)

	err := worker.msgStore.EnsurePartition(expiredTableName)
	if err != nil {
		t.Fatalf("failed to create expired partition: %v", err)
	}

	validDate := time.Now().UTC()
	validTableName := MessageTableNameByDate(validDate)

	err = worker.msgStore.EnsurePartition(validTableName)
	if err != nil {
		t.Fatalf("failed to create valid partition: %v", err)
	}

	var countBefore int
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_inherits 
		WHERE inhparent = 'messages'::regclass`).Scan(&countBefore)
	if err != nil {
		t.Fatalf("failed to count partitions: %v", err)
	}

	if countBefore < 2 {
		t.Fatalf("expected at least 2 partitions, got %d", countBefore)
	}

	worker.dropExpiredPartitions()

	var exists bool
	err = db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`, expiredTableName).Scan(&exists)

	if err != nil {
		t.Errorf("failed to check expired table: %v", err)
	}

	if exists {
		t.Errorf("expired table %s was not dropped", expiredTableName)
	}

	err = db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`, validTableName).Scan(&exists)

	if err != nil {
		t.Errorf("failed to check valid table: %v", err)
	}

	if !exists {
		t.Errorf("valid table %s was incorrectly dropped", validTableName)
	}
}

func TestWorker_StartStop(t *testing.T) {
	worker, _, _ := setupTestWorker(t)

	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.Start()
	}()

	time.Sleep(200 * time.Millisecond)

	stopDone := make(chan struct{})
	go func() {
		defer close(stopDone)
		worker.Stop()
	}()

	select {
	case <-stopDone:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop within timeout")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker goroutines did not complete")
	}
}
