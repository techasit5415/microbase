package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	authOriginMu   sync.RWMutex
	authOriginPool *pgxpool.Pool
)

// SetAuthOriginPool configures the PostgreSQL pool used to persist login origin fingerprints.
func SetAuthOriginPool(pool *pgxpool.Pool) {
	authOriginMu.Lock()
	authOriginPool = pool
	authOriginMu.Unlock()
}

// ComputeAuthFingerprint derives a deterministic SHA-256 hex fingerprint from IP + User-Agent.
func ComputeAuthFingerprint(ip, userAgent string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(ip) + "|" + strings.TrimSpace(userAgent)))
	return hex.EncodeToString(sum[:])[:40]
}

// RecordAuthOrigin asynchronously records or refreshes a login origin fingerprint in _auth_origins.
func RecordAuthOrigin(collectionRef, recordRef, ip, userAgent string) {
	if strings.TrimSpace(collectionRef) == "" || strings.TrimSpace(recordRef) == "" {
		return
	}
	authOriginMu.RLock()
	pool := authOriginPool
	authOriginMu.RUnlock()
	if pool == nil {
		return
	}

	fp := ComputeAuthFingerprint(ip, userAgent)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `
			INSERT INTO _auth_origins (collection_ref, record_ref, fingerprint, ip, user_agent, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
			ON CONFLICT (collection_ref, record_ref, fingerprint)
			DO UPDATE SET ip = EXCLUDED.ip, user_agent = EXCLUDED.user_agent, updated_at = NOW()`,
			collectionRef, recordRef, fp, strings.TrimSpace(ip), strings.TrimSpace(userAgent),
		)
	}()
}
