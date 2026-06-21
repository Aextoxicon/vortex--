package main

import (
	"strings"
	"testing"
)

func TestService_GenerateGroupID(t *testing.T) {
	svc := &Service{
		cfg: &Config{
			GroupIDRandomLength: 8,
		},
	}

	const allowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	t.Run("format and length", func(t *testing.T) {
		id := svc.GenerateGroupID()
		if len(id) != 10 {
			t.Errorf("GenerateGroupID() length = %d, want 10", len(id))
		}
		if !strings.HasPrefix(id, "g_") {
			t.Errorf("GenerateGroupID() = %q, want prefix 'g_'", id)
		}
		for _, c := range id[2:] {
			if !strings.ContainsRune(allowedChars, c) {
				t.Errorf("GenerateGroupID() = %q contains invalid char %c", id, c)
			}
		}
	})

	t.Run("uniqueness", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := svc.GenerateGroupID()
			if seen[id] {
				t.Errorf("GenerateGroupID() generated duplicate %q", id)
				break
			}
			seen[id] = true
		}
	})

	t.Run("custom length", func(t *testing.T) {
		svc12 := &Service{cfg: &Config{GroupIDRandomLength: 12}}
		id := svc12.GenerateGroupID()
		if len(id) != 14 {
			t.Errorf("GenerateGroupID() with length 12 = %q, length = %d, want 14", id, len(id))
		}
	})

	t.Run("zero length uses default", func(t *testing.T) {
		svc0 := &Service{cfg: &Config{GroupIDRandomLength: 0}}
		id := svc0.GenerateGroupID()
		if len(id) != 10 {
			t.Errorf("GenerateGroupID() with length 0 = %q, length = %d, want 10", id, len(id))
		}
	})
}

func TestService_IsValidGroupID(t *testing.T) {
	svc := &Service{
		cfg: &Config{
			GroupIDRandomLength: 8,
		},
	}

	tests := []struct {
		name    string
		groupID string
		want    bool
	}{
		{"valid 10 chars", "g_abc12345", true},
		{"valid alphanumeric", "g_ABCdef12", true},
		{"too short", "g_abc", false},
		{"too long", "g_abcdefghijk", false},
		{"no prefix", "abc1234567", false},
		{"wrong prefix", "x_abc12345", false},
		{"empty string", "", false},
		{"only prefix", "g_", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := svc.IsValidGroupID(tt.groupID); got != tt.want {
				t.Errorf("IsValidGroupID(%q) = %v, want %v", tt.groupID, got, tt.want)
			}
		})
	}
}

func TestService_IsValidGroupID_CustomLength(t *testing.T) {
	svc := &Service{
		cfg: &Config{
			GroupIDRandomLength: 12,
		},
	}

	if got := svc.IsValidGroupID("g_abcdefghijkl"); got != true {
		t.Errorf("IsValidGroupID(14 char) = %v, want true", got)
	}
	if got := svc.IsValidGroupID("g_abcdefghijk"); got != false {
		t.Errorf("IsValidGroupID(13 char) = %v, want false", got)
	}
}

func TestIsGroupConv(t *testing.T) {
	tests := []struct {
		name   string
		convID string
		want   bool
	}{
		{"group conv", "g_abc123", true},
		{"private conv", "p_user1_user2", false},
		{"empty", "", false},
		{"short prefix only", "g_", true},
		{"different prefix", "x_abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGroupConv(tt.convID); got != tt.want {
				t.Errorf("IsGroupConv(%q) = %v, want %v", tt.convID, got, tt.want)
			}
		})
	}
}

func TestExtractGroupID(t *testing.T) {
	tests := []struct {
		name   string
		convID string
		want   string
	}{
		{"normal group", "g_abc123", "abc123"},
		{"nested group id", "g_group_123", "group_123"},
		{"not a group", "p_user1_user2", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractGroupID(tt.convID); got != tt.want {
				t.Errorf("ExtractGroupID(%q) = %q, want %q", tt.convID, got, tt.want)
			}
		})
	}
}
