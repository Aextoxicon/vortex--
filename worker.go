package main

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Worker struct {
	svc      *Service
	msgStore *MessageStore
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

func NewWorker(svc *Service, msgStore *MessageStore) *Worker {
	return &Worker{
		svc:      svc,
		msgStore: msgStore,
		stopCh:   make(chan struct{}),
	}
}

func (w *Worker) Start() {
	w.wg.Add(2)
	go w.runTableManager()
	go w.runMaintenance()
	log.Println("Worker started")
}

func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
	log.Println("Worker stopped")
}

// ==================== TableManager ====================

func (w *Worker) runTableManager() {
	defer w.wg.Done()

	w.createTablesFromTodayToSunday()

	nextMonday := calculateNextMondayDelay()
	select {
	case <-time.After(nextMonday):
	case <-w.stopCh:
		return
	}

	ticker := time.NewTicker(time.Duration(Cfg.WorkerTableCreateIntervalHours) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.createWeekTables()
		case <-w.stopCh:
			return
		}
	}
}

func (w *Worker) createTablesFromTodayToSunday() {
	now := time.Now().UTC()
	dayOfWeek := int(now.Weekday())
	daysToSunday := 0
	if dayOfWeek != 0 {
		daysToSunday = 7 - dayOfWeek
	}

	for offset := 0; offset <= daysToSunday; offset++ {
		date := now.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)
		if _, err := w.msgStore.CreateMessageTable(tableName); err != nil {
			log.Printf("failed to create table %s: %v", tableName, err)
		}
	}
	log.Println("initial message tables created")
}

func (w *Worker) createWeekTables() {
	now := time.Now().UTC()
	for offset := 0; offset < 7; offset++ {
		date := now.AddDate(0, 0, offset)
		tableName := MessageTableNameByDate(date)
		if _, err := w.msgStore.CreateMessageTable(tableName); err != nil {
			log.Printf("failed to create table %s: %v", tableName, err)
		}
	}
	log.Println("weekly message tables created")
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
	defer w.wg.Done()

	select {
	case <-time.After(time.Duration(Cfg.WorkerMaintenanceInitialDelayMinutes) * time.Minute):
	case <-w.stopCh:
		return
	}

	ticker := time.NewTicker(time.Duration(Cfg.WorkerMaintenanceIntervalHours) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.runAnalyze()
			w.dropExpiredPartitions()
		case <-w.stopCh:
			return
		}
	}
}

func (w *Worker) runAnalyze() {
	_, err := w.msgStore.DB().Exec("ANALYZE messages")
	if err != nil {
		log.Printf("maintenance: ANALYZE failed: %v", err)
		return
	}
	log.Println("maintenance: ANALYZE completed")
}

func (w *Worker) dropExpiredPartitions() {
	retentionDays := Cfg.MessageRetentionDays
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	rows, err := w.msgStore.DB().Query(`
		SELECT inhrelid::regclass::text
		FROM pg_inherits
		WHERE inhparent = 'messages'::regclass
	`)
	if err != nil {
		log.Printf("maintenance: failed to list partitions: %v", err)
		return
	}
	defer rows.Close()

	var partitions []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			log.Printf("maintenance: failed to scan partition name: %v", err)
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
				log.Printf("maintenance: failed to quote partition name %s: %v", partition, err)
				continue
			}
			_, err = w.msgStore.DB().Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted))
			if err != nil {
				log.Printf("maintenance: failed to drop partition %s: %v", partition, err)
				continue
			}
			log.Printf("maintenance: dropped partition %s", partition)
		}
	}
	log.Println("maintenance: drop expired partitions completed")
}
