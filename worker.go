package main

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Worker struct {
	cfg      *Config
	svc      *Service
	msgStore *MessageStore
	stopCh   chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	started  bool
}

func NewWorker(cfg *Config, svc *Service, msgStore *MessageStore) *Worker {
	return &Worker{
		cfg:      cfg,
		svc:      svc,
		msgStore: msgStore,
		stopCh:   make(chan struct{}),
	}
}

func (w *Worker) Start() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return
	}
	w.started = true

	w.wg.Add(3)
	go w.runTableManager()
	go w.runMaintenance()
	go w.runIdempotencyCleanup()
	slog.Info("Worker started")
}

func (w *Worker) Stop() {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return
	}
	w.started = false
	w.mu.Unlock()

	close(w.stopCh)
	w.wg.Wait()
	slog.Info("Worker stopped")
}

// runLoop 通用轮询循环：等待初始延迟 → 定时执行 fn
func (w *Worker) runLoop(delay time.Duration, interval time.Duration, fn func()) {
	defer w.wg.Done()

	select {
	case <-time.After(delay):
	case <-w.stopCh:
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fn()
		case <-w.stopCh:
			return
		}
	}
}

// ==================== TableManager ====================

func (w *Worker) runTableManager() {
	w.runLoop(calculateNextMondayDelay(), time.Duration(w.cfg.WorkerTableCreateIntervalHours)*time.Hour, w.createWeekTables)
}

func (w *Worker) createTablesFromTodayToSunday() error {
	now := time.Now().UTC()
	dayOfWeek := int(now.Weekday())
	daysToSunday := 0
	if dayOfWeek != 0 {
		daysToSunday = 7 - dayOfWeek
	}

	var lastErr error
	for offset := 0; offset <= daysToSunday; offset++ {
		date := now.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)
		if err := w.msgStore.EnsurePartition(tableName); err != nil {
			slog.Error("failed to create table", "table", tableName, "error", err)
			lastErr = err
		}
	}
	slog.Info("initial message tables created")
	return lastErr
}

// CreateTablesFromTodayToSundayWithError 带错误返回的版本，用于启动时检查
func (w *Worker) CreateTablesFromTodayToSundayWithError() error {
	return w.createTablesFromTodayToSunday()
}

func (w *Worker) createWeekTables() {
	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	nextMonday := now.AddDate(0, 0, 8-weekday)

	for offset := 0; offset < 7; offset++ {
		date := nextMonday.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)
		if err := w.msgStore.EnsurePartition(tableName); err != nil {
			slog.Error("failed to create table", "table", tableName, "error", err)
		}
	}
	slog.Info("weekly message tables created")
}

func calculateNextMondayDelay() time.Duration {
	now := time.Now().UTC()
	daysUntilMonday := (8 - int(now.Weekday())) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	nextMonday := time.Date(now.Year(), now.Month(), now.Day()+daysUntilMonday,
		0, 0, 0, 0, time.UTC)
	return nextMonday.Sub(now)
}

// ==================== Maintenance ====================

func (w *Worker) runMaintenance() {
	w.runLoop(time.Duration(w.cfg.WorkerMaintenanceInitialDelayMinutes)*time.Minute,
		time.Duration(w.cfg.WorkerMaintenanceIntervalHours)*time.Hour,
		func() { w.runAnalyze(); w.dropExpiredPartitions() })
}

func (w *Worker) runAnalyze() {
	_, err := w.msgStore.DB().Exec("ANALYZE messages")
	if err != nil {
		slog.Error("maintenance: ANALYZE failed", "error", err)
		return
	}
	slog.Info("maintenance: ANALYZE completed")
}

func (w *Worker) dropExpiredPartitions() {
	retentionDays := w.cfg.MessageRetentionDays
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	rows, err := w.msgStore.DB().Query(`
		SELECT inhrelid::regclass::text
		FROM pg_inherits
		WHERE inhparent = 'messages'::regclass
	`)
	if err != nil {
		slog.Error("maintenance: failed to list partitions", "error", err)
		return
	}
	defer rows.Close()

	var partitions []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			slog.Error("maintenance: failed to scan partition name", "error", err)
			continue
		}
		partitions = append(partitions, name)
	}

	for _, partition := range partitions {
		if len(partition) < 10 || partition[:9] != "messages_" {
			continue
		}
		dateStr := partition[9:]
		partitionDate, err := time.Parse("20060102", dateStr)
		if err != nil {
			continue
		}
		if partitionDate.Before(cutoff) {
			var quoted string
			err := w.msgStore.DB().QueryRow("SELECT quote_ident($1)", partition).Scan(&quoted)
			if err != nil {
				slog.Error("maintenance: failed to quote partition name", "partition", partition, "error", err)
				continue
			}
			_, err = w.msgStore.DB().Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted))
			if err != nil {
				slog.Error("maintenance: failed to drop partition", "partition", partition, "error", err)
				continue
			}
			slog.Info("maintenance: dropped partition", "partition", partition)
		}
	}
	slog.Info("maintenance: drop expired partitions completed")
}

func MessageTableNameByDate(date time.Time) string {
	return fmt.Sprintf("messages_%s", date.Format("20060102"))
}

func MessageTableNameByTs(ts int64) string {
	t := time.UnixMilli(ts)
	return MessageTableNameByDate(t)
}

// ==================== Idempotency Cleanup ====================

func (w *Worker) runIdempotencyCleanup() {
	w.runLoop(time.Duration(w.cfg.WorkerMaintenanceInitialDelayMinutes)*time.Minute,
		time.Duration(w.cfg.WorkerMaintenanceIntervalHours)*time.Hour,
		w.cleanupIdempotency)
}

func (w *Worker) cleanupIdempotency() {
	retentionHours := int64(24) // 默认保留24小时
	cutoff := time.Now().UnixMilli() - retentionHours*3600*1000
	result, err := w.msgStore.DB().Exec(`DELETE FROM message_idempotency WHERE created_at < $1`, cutoff)
	if err != nil {
		slog.Error("idempotency cleanup failed", "error", err)
		return
	}
	count, _ := result.RowsAffected()
	if count > 0 {
		slog.Info("cleaned up stale idempotency records", "count", count)
	}
}
