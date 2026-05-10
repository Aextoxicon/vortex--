package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"sync"
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
	blacklistMu      sync.RWMutex
	blacklistCache   map[string]int64
	stopCh           chan struct{}
}

func NewJwtService(db *sql.DB, secret, issuer string, expiresInMinutes int) *JwtService {
	js := &JwtService{
		db:               db,
		secret:           []byte(secret),
		issuer:           issuer,
		expiresInMinutes: expiresInMinutes,
		blacklistCache:   make(map[string]int64),
		stopCh:           make(chan struct{}),
	}
	js.loadBlacklistFromDB()
	return js
}

func (j *JwtService) loadBlacklistFromDB() {
	rows, err := j.db.Query(`SELECT jti, expires_at FROM jwt_blacklist`)
	if err != nil {
		return
	}
	defer rows.Close()

	now := time.Now().UnixMilli()
	j.blacklistMu.Lock()
	defer j.blacklistMu.Unlock()

	for rows.Next() {
		var jti string
		var expiresAt int64
		if err := rows.Scan(&jti, &expiresAt); err != nil {
			continue
		}
		if expiresAt > now {
			j.blacklistCache[jti] = expiresAt
		}
	}
}

func (j *JwtService) BlacklistToken(jti string, expiresAt int64) error {
	_, err := j.db.Exec(`
		INSERT INTO jwt_blacklist (jti, expires_at)
		VALUES ($1, $2)
		ON CONFLICT (jti) DO NOTHING`,
		jti, expiresAt,
	)
	if err == nil {
		j.blacklistMu.Lock()
		j.blacklistCache[jti] = expiresAt
		j.blacklistMu.Unlock()
	}
	return err
}

func (j *JwtService) IsBlacklisted(jti string) bool {
	j.blacklistMu.RLock()
	exp, ok := j.blacklistCache[jti]
	j.blacklistMu.RUnlock()
	return ok && exp > time.Now().UnixMilli()
}

func (j *JwtService) CleanupBlacklist() {
	now := time.Now().UnixMilli()

	_, _ = j.db.Exec(`DELETE FROM jwt_blacklist WHERE expires_at < $1`, now)

	j.blacklistMu.Lock()
	defer j.blacklistMu.Unlock()
	for jti, exp := range j.blacklistCache {
		if exp < now {
			delete(j.blacklistCache, jti)
		}
	}
}

func (j *JwtService) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				j.CleanupBlacklist()
			case <-j.stopCh:
				return
			}
		}
	}()
}

func (j *JwtService) Stop() {
	close(j.stopCh)
}

func (j *JwtService) GenerateToken(user *User) (string, error) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(j.expiresInMinutes) * time.Minute)

	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("failed to generate token ID: %w", err)
	}
	jti := fmt.Sprintf("%x", jtiBytes)

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
			ID:        jti,
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
