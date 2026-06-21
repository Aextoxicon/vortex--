package main

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestValidateCredentials(t *testing.T) {
	password := "Test1234!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	svc := &Service{}
	user := &User{PwdHash: string(hash)}

	t.Run("correct password", func(t *testing.T) {
		if err := svc.ValidateCredentials(user, password); err != nil {
			t.Errorf("ValidateCredentials() with correct password error = %v", err)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		if err := svc.ValidateCredentials(user, "WrongPassword1!"); err != ErrInvalidCredentials {
			t.Errorf("ValidateCredentials() with wrong password error = %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("empty password", func(t *testing.T) {
		if err := svc.ValidateCredentials(user, ""); err != ErrInvalidCredentials {
			t.Errorf("ValidateCredentials() with empty password error = %v, want ErrInvalidCredentials", err)
		}
	})
}

func TestValidateCredentials_MultipleUsers(t *testing.T) {
	// Verify each user's password is independent
	hash1, _ := bcrypt.GenerateFromPassword([]byte("Password1!"), bcrypt.DefaultCost)
	hash2, _ := bcrypt.GenerateFromPassword([]byte("Secret2@"), bcrypt.DefaultCost)

	user1 := &User{PwdHash: string(hash1)}
	user2 := &User{PwdHash: string(hash2)}

	svc := &Service{}

	if err := svc.ValidateCredentials(user1, "Password1!"); err != nil {
		t.Error("user1 correct password should pass")
	}
	if err := svc.ValidateCredentials(user1, "Secret2@"); err != ErrInvalidCredentials {
		t.Error("user2's password should not work for user1")
	}
	if err := svc.ValidateCredentials(user2, "Secret2@"); err != nil {
		t.Error("user2 correct password should pass")
	}
}
