package main

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes uint64
		want  string
	}{
		{"zero bytes", 0, "0.0 KB"},
		{"1 KB", 1024, "1.0 KB"},
		{"500 KB", 500 * 1024, "500.0 KB"},
		{"1023 KB", 1023 * 1024, "1023.0 KB"},
		{"1 MB", 1024 * 1024, "1.0 MB"},
		{"512 MB", 512 * 1024 * 1024, "512.0 MB"},
		{"1023 MB", 1023 * 1024 * 1024, "1023.0 MB"},
		{"1 GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"1.5 GB", 1500 * 1024 * 1024 * 1024, "1500.0 GB"},
		{"1024 GB", 1024 * 1024 * 1024 * 1024, "1024.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatBytes(tt.bytes); got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatBytes_EdgeCases(t *testing.T) {
	// Just under 1 MB boundary
	got := formatBytes(1024*1024 - 1)
	if got != "1024.0 KB" {
		t.Errorf("formatBytes(1024*1024-1) = %q, want %q", got, "1024.0 KB")
	}

	// Exactly at 1 MB boundary
	got = formatBytes(1024 * 1024)
	if got != "1.0 MB" {
		t.Errorf("formatBytes(1024*1024) = %q, want %q", got, "1.0 MB")
	}

	// Just under 1 GB boundary
	got = formatBytes(1024*1024*1024 - 1)
	if got != "1024.0 MB" {
		t.Errorf("formatBytes(1024*1024*1024-1) = %q, want %q", got, "1024.0 MB")
	}

	// Exactly at 1 GB boundary
	got = formatBytes(1024 * 1024 * 1024)
	if got != "1.0 GB" {
		t.Errorf("formatBytes(1024*1024*1024) = %q, want %q", got, "1.0 GB")
	}
}
