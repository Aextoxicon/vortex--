package main

import (
	"testing"
)

func TestExtractFileKeyFromMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "image message with file key",
			content: `{"type":"image","content":"uploads/p_abc_def/nanoid123.jpg","text":"hello"}`,
			want:    "uploads/p_abc_def/nanoid123.jpg",
		},
		{
			name:    "image message without content",
			content: `{"type":"image","content":"","text":"hello"}`,
			want:    "",
		},
		{
			name:    "text message (not json)",
			content: "hello world",
			want:    "",
		},
		{
			name:    "text message (plain json but not image)",
			content: `{"type":"text","content":"hello"}`,
			want:    "",
		},
		{
			name:    "empty content",
			content: "",
			want:    "",
		},
		{
			name:    "malformed json",
			content: `{"type":"image","content":`,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{Content: tt.content}
			if got := extractFileKeyFromMessage(msg); got != tt.want {
				t.Errorf("extractFileKeyFromMessage(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}
