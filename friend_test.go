package main

import (
	"testing"
)

func TestPrivateConvID(t *testing.T) {
	tests := []struct {
		name      string
		publicID1 string
		publicID2 string
		want      string
	}{
		{"first smaller", "aaa", "bbb", "p_aaa_bbb"},
		{"second smaller", "bbb", "aaa", "p_aaa_bbb"},
		{"equal IDs", "aaa", "aaa", "p_aaa_aaa"},
		{"numeric IDs", "user123", "user456", "p_user123_user456"},
		{"alphanumeric", "abc123", "def456", "p_abc123_def456"},
		{"empty string", "", "bbb", "p__bbb"},
		{"both empty", "", "", "p__"},
		{"with special chars", "test_user", "other_user", "p_other_user_test_user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PrivateConvID(tt.publicID1, tt.publicID2); got != tt.want {
				t.Errorf("PrivateConvID(%q, %q) = %q, want %q", tt.publicID1, tt.publicID2, got, tt.want)
			}
		})
	}
}

func TestPrivateConvID_Deterministic(t *testing.T) {
	// Same pair should always produce the same result regardless of order
	id1 := PrivateConvID("testuser", "otheruser")
	id2 := PrivateConvID("otheruser", "testuser")
	if id1 != id2 {
		t.Errorf("PrivateConvID not deterministic: %q vs %q", id1, id2)
	}
}
