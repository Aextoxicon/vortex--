package main

import (
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{"valid password", "Test1234!", true},
		{"too short", "Test1!", false},
		{"too long", "Test1234!Test1234!Test", false},
		{"no uppercase", "test1234!", false},
		{"no lowercase", "TEST1234!", false},
		{"no digit", "TestTest!", false},
		{"no special char", "Test1234", false},
		{"empty", "", false},
		{"only spaces", "        ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validatePassword(tt.password); got != tt.want {
				t.Errorf("validatePassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     bool
	}{
		{"valid username", "testuser", true},
		{"valid with underscore", "test_user", true},
		{"valid with number", "test123", true},
		{"too short", "ab", false},
		{"too long", "thisusernameiswaytoolongandexceedslimit", false},
		{"with special char", "test@user", false},
		{"empty", "", false},
		{"with space", "test user", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateUsername(tt.username); got != tt.want {
				t.Errorf("validateUsername(%q) = %v, want %v", tt.username, got, tt.want)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"valid email", "test@example.com", true},
		{"valid with subdomain", "test@mail.example.com", true},
		{"valid with plus", "test+tag@example.com", true},
		{"no @ symbol", "testexample.com", false},
		{"no domain", "test@", false},
		{"no TLD", "test@example", false},
		{"empty", "", true}, // empty is allowed (optional field)
		{"invalid format", "test@.com", false},
		{"too long", "test@example.com/verylongsubdomainthatexceedsthelimit", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateEmail(tt.email); got != tt.want {
				t.Errorf("validateEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Test Group", true},
		{"Test_Group", true},
		{"Test-Group", true},
		{"Group123", true},
		{"This group name is way too long and exceeds the fifty character limit", false},
		{"", false},
		{"Group@123", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := validateGroupName(tt.input); got != tt.want {
				t.Errorf("validateGroupName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
