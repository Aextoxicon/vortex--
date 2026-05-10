package main

import (
	"testing"
	"time"
)

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
