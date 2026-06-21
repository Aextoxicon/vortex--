package main

import (
	"testing"
)

func TestGenerateFileKey(t *testing.T) {
	tests := []struct {
		name    string
		convID  string
		fileExt string
	}{
		{"normal jpg", "p_abc_def", "jpg"},
		{"png image", "g_group123", "png"},
		{"no extension", "p_abc_def", ""},
		{"long convID", "p_very_long_public_id_12345", "pdf"},
		{"numeric convID", "p_user1_user2", "txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := GenerateFileKey(tt.convID, tt.fileExt)

			expectedPrefix := "uploads/" + tt.convID + "/"
			if len(key) <= len(expectedPrefix) {
				t.Errorf("GenerateFileKey(%q, %q) = %q, too short", tt.convID, tt.fileExt, key)
				return
			}
			if key[:len(expectedPrefix)] != expectedPrefix {
				t.Errorf("GenerateFileKey(%q, %q) = %q, want prefix %q", tt.convID, tt.fileExt, key, expectedPrefix)
			}

			suffix := "." + tt.fileExt
			if tt.fileExt != "" {
				if len(key) < len(suffix) || key[len(key)-len(suffix):] != suffix {
					t.Errorf("GenerateFileKey(%q, %q) = %q, want suffix %q", tt.convID, tt.fileExt, key, suffix)
				}
			} else {
				if len(key) > 0 && key[len(key)-1:] == "." {
					t.Errorf("GenerateFileKey(%q, %q) = %q, unexpected trailing dot", tt.convID, tt.fileExt, key)
				}
			}

			idPart := key[len(expectedPrefix) : len(key)-len(tt.fileExt)-1]
			if len(idPart) != 21 {
				t.Errorf("GenerateFileKey(%q, %q) = %q, nanoid part length = %d, want 21",
					tt.convID, tt.fileExt, key, len(idPart))
			}
		})
	}
}

func TestGenerateFileKey_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key := GenerateFileKey("p_test", "jpg")
		if seen[key] {
			t.Errorf("duplicate file key generated: %q", key)
			break
		}
		seen[key] = true
	}
}

func TestExtractConvIDFromKey(t *testing.T) {
	tests := []struct {
		name    string
		fileKey string
		want    string
		wantErr bool
	}{
		{"valid private conv", "uploads/p_abc_def/abc123.jpg", "p_abc_def", false},
		{"valid group upload", "uploads/g_group1/file.pdf", "g_group1", false},
		{"numeric conv id", "uploads/p_123_456/nanoid.txt", "p_123_456", false},
		{"no prefix dir", "invalid/p_abc/file.txt", "", true},
		{"too few parts", "invalid", "", true},
		{"empty key", "", "", true},
		{"wrong root dir", "downloads/p_abc/file.txt", "", true},
		{"extra deep path", "uploads/a/b/c/file.txt", "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractConvIDFromKey(tt.fileKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractConvIDFromKey(%q) error = %v, wantErr = %v", tt.fileKey, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractConvIDFromKey(%q) = %q, want %q", tt.fileKey, got, tt.want)
			}
		})
	}
}
