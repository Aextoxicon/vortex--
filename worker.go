package main

import (
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

	ticker := time.NewTicker(7 * 24 * time.Hour)
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
	case <-time.After(5 * time.Minute):
	case <-w.stopCh:
		return
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			log.Println("maintenance: running ANALYZE")
		case <-w.stopCh:
			return
		}
	}
}
