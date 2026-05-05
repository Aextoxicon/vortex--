package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JwtClaims struct {
	UserID   int64  `json:"sub"`
	PublicID string `json:"public_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type JwtService struct {
	db               *sql.DB
	secret           []byte
	issuer           string
	expiresInMinutes int
}

func NewJwtService(db *sql.DB, secret, issuer string, expiresInMinutes int) *JwtService {
	return &JwtService{
		db:               db,
		secret:           []byte(secret),
		issuer:           issuer,
		expiresInMinutes: expiresInMinutes,
	}
}

func (j *JwtService) BlacklistToken(jti string, expiresAt int64) error {
	_, err := j.db.Exec(`
		INSERT INTO jwt_blacklist (jti, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (jti) DO NOTHING`,
		jti, expiresAt,
	)
	return err
}

func (j *JwtService) IsBlacklisted(jti string) bool {
	var exists bool
	err := j.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM jwt_blacklist WHERE jti = $1)`,
		jti,
	).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

func (j *JwtService) CleanupBlacklist() {
	now := time.Now().UnixMilli()
	_, err := j.db.Exec(`DELETE FROM jwt_blacklist WHERE expires_at < $1`, now)
	if err != nil {
		return
	}
}

func (j *JwtService) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			j.CleanupBlacklist()
		}
	}()
}

func (j *JwtService) GenerateToken(user *User) (string, error) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(j.expiresInMinutes) * time.Minute)
	claims := JwtClaims{
		UserID:   user.ID,
		PublicID: user.PublicID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    j.issuer,
			Audience:  []string{j.issuer},
			ID:        fmt.Sprintf("%d", now.UnixNano()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

func (j *JwtService) ValidateToken(tokenStr string) (*JwtClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	}, jwt.WithIssuer(j.issuer), jwt.WithAudience(j.issuer))
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*JwtClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
