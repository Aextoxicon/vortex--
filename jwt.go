package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/lib/pq"
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

	// 策略：先快速扫描收集过期项（持有读锁），再批量删除（持有写锁）
	// 这样减少写锁持有时间，提高并发性能

	// 第1步：快速扫描，收集过期项（读锁，不阻塞其他读操作）
	j.blacklistMu.RLock()
	expiredItems := make([]string, 0, 100) // 预分配容量，避免频繁扩容
	for jti, exp := range j.blacklistCache {
		if exp < now {
			expiredItems = append(expiredItems, jti)
			if len(expiredItems) >= 1000 {
				// 限制单次处理数量，避免内存占用过大
				break
			}
		}
	}
	j.blacklistMu.RUnlock()

	// 第2步：批量删除（写锁，但持有时间很短）
	if len(expiredItems) > 0 {
		j.blacklistMu.Lock()
		// 双重检查：删除前再次确认是否过期
		// 防止在 RUnlock 和 Lock 之间被其他 goroutine 修改
		for _, jti := range expiredItems {
			if exp, ok := j.blacklistCache[jti]; ok && exp < now {
				delete(j.blacklistCache, jti)
			}
		}
		j.blacklistMu.Unlock()

		// 第3步：异步清理数据库（慢操作，不阻塞）
		go func(items []string) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// 使用批量删除，提高效率
			if len(items) == 1 {
				_, _ = j.db.ExecContext(ctx, `DELETE FROM jwt_blacklist WHERE jti = $1`, items[0])
			} else {
				// PostgreSQL 支持批量删除
				_, _ = j.db.ExecContext(ctx,
					`DELETE FROM jwt_blacklist WHERE jti = ANY($1)`,
					pq.Array(items))
			}
		}(expiredItems)
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
