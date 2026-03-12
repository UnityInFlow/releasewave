package tenant

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// APIKey represents a tenant API key.
type APIKey struct {
	ID        int64     `json:"id"`
	TenantID  int64     `json:"tenant_id"`
	Prefix    string    `json:"prefix"` // First 8 chars for identification
	Hash      string    `json:"-"`      // SHA-256 hash of full key
	CreatedAt time.Time `json:"created_at"`
}

// KeyStore manages API keys backed by SQLite.
type KeyStore struct {
	db *sql.DB
}

// NewKeyStore creates a key store and runs migrations.
func NewKeyStore(db *sql.DB) (*KeyStore, error) {
	ks := &KeyStore{db: db}
	if err := ks.migrate(); err != nil {
		return nil, err
	}
	return ks, nil
}

func (ks *KeyStore) migrate() error {
	if _, err := ks.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	_, err := ks.db.Exec(`CREATE TABLE IF NOT EXISTS api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		prefix TEXT NOT NULL,
		hash TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
	)`)
	return err
}

// Generate creates a new API key for a tenant. Returns the raw key (only shown once).
func (ks *KeyStore) Generate(tenantID int64) (rawKey string, key *APIKey, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate key: %w", err)
	}

	raw := "rw_" + hex.EncodeToString(b)
	prefix := raw[:11] // "rw_" + 8 hex chars
	hash := hashKey(raw)

	result, err := ks.db.Exec(
		`INSERT INTO api_keys (tenant_id, prefix, hash) VALUES (?, ?, ?)`,
		tenantID, prefix, hash,
	)
	if err != nil {
		return "", nil, fmt.Errorf("store key: %w", err)
	}

	id, _ := result.LastInsertId()
	return raw, &APIKey{
		ID:        id,
		TenantID:  tenantID,
		Prefix:    prefix,
		Hash:      hash,
		CreatedAt: time.Now(),
	}, nil
}

// Validate checks if a raw API key is valid and returns the associated tenant ID.
func (ks *KeyStore) Validate(rawKey string) (int64, error) {
	hash := hashKey(rawKey)
	var tenantID int64
	err := ks.db.QueryRow(
		`SELECT tenant_id FROM api_keys WHERE hash = ?`, hash,
	).Scan(&tenantID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("invalid API key")
	}
	if err != nil {
		return 0, err
	}
	return tenantID, nil
}

// ListForTenant returns all API keys for a tenant (without hashes).
func (ks *KeyStore) ListForTenant(tenantID int64) ([]APIKey, error) {
	rows, err := ks.db.Query(
		`SELECT id, tenant_id, prefix, created_at FROM api_keys WHERE tenant_id = ? ORDER BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Prefix, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Revoke deletes an API key by ID.
func (ks *KeyStore) Revoke(keyID int64) error {
	result, err := ks.db.Exec(`DELETE FROM api_keys WHERE id = ?`, keyID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("key not found")
	}
	return nil
}

func hashKey(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
