package main

import (
	"testing"
	"time"
)

// 和上面的xxx测试一样，测试jwt的
func TestJwtService_GenerateAndValidate(t *testing.T) {
	_, db, _ := setupTestStore(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	js := NewJwtService(db, "test-secret-key-min-32-chars-long!!", "test-issuer", 60)
	defer js.Stop()

	user := &User{
		ID:       123,
		Username: "testuser",
		PublicID: "pub123",
	}

	token, err := js.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := js.ValidateToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UserID != user.ID {
		t.Errorf("expected user ID %d, got %d", user.ID, claims.UserID)
	}

	if claims.Username != user.Username {
		t.Errorf("expected username %s, got %s", user.Username, claims.Username)
	}

	if claims.PublicID != user.PublicID {
		t.Errorf("expected public ID %s, got %s", user.PublicID, claims.PublicID)
	}

	if claims.Issuer != "test-issuer" {
		t.Errorf("expected issuer test-issuer, got %s", claims.Issuer)
	}
}

func TestJwtService_Blacklist(t *testing.T) {
	_, db, _ := setupTestStore(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	js := NewJwtService(db, "test-secret-key-min-32-chars-long!!", "test-issuer", 60)
	defer js.Stop()

	expiresAt := time.Now().Add(time.Hour).UnixMilli()

	// Add to blacklist
	err := js.BlacklistToken("test-jti", expiresAt)
	if err != nil {
		t.Fatalf("failed to blacklist token: %v", err)
	}

	// Verify it's blacklisted
	if !js.IsBlacklisted("test-jti") {
		t.Error("expected token to be blacklisted")
	}

	// Verify other token is not blacklisted
	if js.IsBlacklisted("other-jti") {
		t.Error("expected other token to not be blacklisted")
	}
}

func TestJwtService_Cleanup(t *testing.T) {
	_, db, _ := setupTestStore(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	js := NewJwtService(db, "test-secret-key-min-32-chars-long!!", "test-issuer", 60)
	defer js.Stop()

	// Add an expired blacklist entry
	expiresAt := time.Now().Add(-time.Hour).UnixMilli()
	err := js.BlacklistToken("expired-jti", expiresAt)
	if err != nil {
		t.Fatalf("failed to blacklist token: %v", err)
	}

	// Cleanup
	js.CleanupBlacklist()

	// Verify it's cleaned up
	if js.IsBlacklisted("expired-jti") {
		t.Error("expected expired token to be cleaned up")
	}
}

func TestJwtService_InvalidToken(t *testing.T) {
	_, db, _ := setupTestStore(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	js := NewJwtService(db, "test-secret-key-min-32-chars-long!!", "test-issuer", 60)
	defer js.Stop()

	// Test invalid token
	_, err := js.ValidateToken("invalid.token.here")
	if err == nil {
		t.Error("expected error for invalid token")
	}

	// Test empty token
	_, err = js.ValidateToken("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestJwtService_WrongSecret(t *testing.T) {
	_, db, _ := setupTestStore(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	js1 := NewJwtService(db, "secret1-key-min-32-chars-long!!", "test-issuer", 60)
	defer js1.Stop()

	js2 := NewJwtService(db, "secret2-key-min-32-chars-long!!", "test-issuer", 60)
	defer js2.Stop()

	user := &User{
		ID:       123,
		Username: "testuser",
		PublicID: "pub123",
	}

	// Generate token with js1
	token, err := js1.GenerateToken(user)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Validate with js2 (different secret) should fail
	_, err = js2.ValidateToken(token)
	if err == nil {
		t.Error("expected error when validating token with wrong secret")
	}
}

func TestJwtService_BlacklistPersistence(t *testing.T) {
	_, db, _ := setupTestStore(t)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	// Create first service and blacklist token
	js1 := NewJwtService(db, "test-secret-key-min-32-chars-long!!", "test-issuer", 60)
	expiresAt := time.Now().Add(time.Hour).UnixMilli()
	err := js1.BlacklistToken("persist-jti", expiresAt)
	if err != nil {
		t.Fatalf("failed to blacklist token: %v", err)
	}
	js1.Stop()

	// Create second service and verify blacklist is loaded from DB
	js2 := NewJwtService(db, "test-secret-key-min-32-chars-long!!", "test-issuer", 60)
	defer js2.Stop()

	if !js2.IsBlacklisted("persist-jti") {
		t.Error("expected blacklist to persist across service restarts")
	}
}
