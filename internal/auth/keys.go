package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ServiceKey struct {
	ID         string
	Prefix     string
	Principal  string
	Scopes     []string
	UseOnce    bool
	ExpiresAt  *time.Time
	Revoked    bool
	CreatedAt  time.Time
	LastUsedAt *time.Time
	UseCount   int
}

type ServiceKeyValidation struct {
	KeyID       string
	PrincipalID string
	Scopes      map[string]struct{}
}

type ServiceKeyStore struct {
	db *sql.DB
}

// NewServiceKeyStore creates the service_api_keys table if it doesn't exist
// and returns a ready-to-use store. It uses the same *sql.DB as the index.
func NewServiceKeyStore(db *sql.DB) (*ServiceKeyStore, error) {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS service_api_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL UNIQUE,
			prefix TEXT NOT NULL,
			principal_id TEXT NOT NULL,
			scopes_json TEXT NOT NULL DEFAULT '[]',
			use_once INTEGER NOT NULL DEFAULT 0,
			expires_at TIMESTAMP NULL,
			revoked INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			last_used_at TIMESTAMP NULL,
			use_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_service_api_keys_prefix ON service_api_keys(prefix)`,
		`CREATE INDEX IF NOT EXISTS idx_service_api_keys_principal ON service_api_keys(principal_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("auth migration: %w", err)
		}
	}
	return &ServiceKeyStore{db: db}, nil
}

func (s *ServiceKeyStore) Issue(ctx context.Context, principal string, scopes []string, ttl time.Duration, useOnce bool) (*ServiceKey, string, error) {
	if strings.TrimSpace(principal) == "" {
		return nil, "", fmt.Errorf("principal is required")
	}
	raw, err := generateRawServiceKey()
	if err != nil {
		return nil, "", err
	}
	now := time.Now().UTC()
	id := "sak_" + fmt.Sprint(now.UnixNano())
	keyHash := hashKey(raw)
	prefix := raw
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	scopes = normalizeScopes(scopes)
	scopesJSON, _ := json.Marshal(scopes)
	var expiresAt any
	var expPtr *time.Time
	if ttl > 0 {
		ts := now.Add(ttl)
		expPtr = &ts
		expiresAt = ts
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO service_api_keys (
			id, key_hash, prefix, principal_id, scopes_json, use_once, expires_at, revoked, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)`,
		id, keyHash, prefix, principal, string(scopesJSON), boolToInt(useOnce), expiresAt, now,
	)
	if err != nil {
		return nil, "", err
	}
	return &ServiceKey{
		ID:        id,
		Prefix:    prefix,
		Principal: principal,
		Scopes:    scopes,
		UseOnce:   useOnce,
		ExpiresAt: expPtr,
		CreatedAt: now,
	}, raw, nil
}

func (s *ServiceKeyStore) Validate(ctx context.Context, rawKey string) (*ServiceKeyValidation, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, fmt.Errorf("key required")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, principal_id, scopes_json, use_once, COALESCE(use_count,0), expires_at, revoked
		FROM service_api_keys WHERE key_hash = ?`, hashKey(rawKey))
	var (
		id, principal, scopesJSON string
		useOnce, useCount, revoked int
		expiresAt                  sql.NullTime
	)
	if err := row.Scan(&id, &principal, &scopesJSON, &useOnce, &useCount, &expiresAt, &revoked); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid key")
		}
		return nil, err
	}
	if revoked != 0 {
		return nil, fmt.Errorf("key revoked")
	}
	now := time.Now().UTC()
	if expiresAt.Valid && now.After(expiresAt.Time) {
		return nil, fmt.Errorf("key expired")
	}
	if useOnce != 0 && useCount > 0 {
		return nil, fmt.Errorf("key already used")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE service_api_keys
		SET last_used_at = ?, use_count = use_count + 1
		WHERE id = ?`, now, id)
	if err != nil {
		return nil, err
	}
	var scopes []string
	_ = json.Unmarshal([]byte(scopesJSON), &scopes)
	return &ServiceKeyValidation{
		KeyID:       id,
		PrincipalID: principal,
		Scopes:      makeSet(scopes),
	}, nil
}

func (s *ServiceKeyStore) Revoke(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE service_api_keys SET revoked = 1 WHERE id = ?`, id)
	return err
}

func (s *ServiceKeyStore) List(ctx context.Context, limit int) ([]ServiceKey, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, prefix, principal_id, scopes_json, use_once, expires_at, revoked, created_at, last_used_at, use_count
		FROM service_api_keys
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ServiceKey, 0, limit)
	for rows.Next() {
		var (
			k         ServiceKey
			scopesRaw string
			useOnce   int
			revoked   int
			expiresAt sql.NullTime
			lastUsed  sql.NullTime
		)
		if err := rows.Scan(
			&k.ID, &k.Prefix, &k.Principal, &scopesRaw, &useOnce, &expiresAt,
			&revoked, &k.CreatedAt, &lastUsed, &k.UseCount,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(scopesRaw), &k.Scopes)
		k.UseOnce = useOnce != 0
		k.Revoked = revoked != 0
		if expiresAt.Valid {
			ts := expiresAt.Time
			k.ExpiresAt = &ts
		}
		if lastUsed.Valid {
			ts := lastUsed.Time
			k.LastUsedAt = &ts
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func generateRawServiceKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "psk_" + hex.EncodeToString(buf), nil
}

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeScopes(scopes []string) []string {
	out := make([]string, 0, len(scopes))
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func makeSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, v := range items {
		m[v] = struct{}{}
	}
	return m
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
