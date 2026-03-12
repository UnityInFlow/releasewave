package tenant

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB returns an in-memory SQLite connection with foreign keys enabled.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// setupStores creates both a Store and a KeyStore on the same in-memory DB.
func setupStores(t *testing.T) (*sql.DB, *Store, *KeyStore) {
	t.Helper()
	db := openTestDB(t)
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ks, err := NewKeyStore(db)
	if err != nil {
		t.Fatalf("new key store: %v", err)
	}
	return db, store, ks
}

// ---------------------------------------------------------------------------
// Tenant Store tests
// ---------------------------------------------------------------------------

func TestStore_CreateAndGet(t *testing.T) {
	_, store, _ := setupStores(t)

	created, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Name != "acme" {
		t.Fatalf("unexpected tenant: %+v", created)
	}

	got, err := store.Get("acme")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("id mismatch: got %d, want %d", got.ID, created.ID)
	}
	if got.Name != "acme" {
		t.Fatalf("name mismatch: got %q, want %q", got.Name, "acme")
	}
}

func TestStore_CreateDuplicateFails(t *testing.T) {
	_, store, _ := setupStores(t)

	if _, err := store.Create("acme"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := store.Create("acme")
	if err == nil {
		t.Fatal("expected error for duplicate tenant, got nil")
	}
}

func TestStore_ListReturnsAll(t *testing.T) {
	_, store, _ := setupStores(t)

	names := []string{"alpha", "beta", "gamma"}
	for _, n := range names {
		if _, err := store.Create(n); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}

	list, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 tenants, got %d", len(list))
	}
	// Store.List orders by name, so alpha, beta, gamma.
	for i, n := range names {
		if list[i].Name != n {
			t.Errorf("list[%d].Name = %q, want %q", i, list[i].Name, n)
		}
	}
}

func TestStore_DeleteSucceeds(t *testing.T) {
	_, store, _ := setupStores(t)

	if _, err := store.Create("acme"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.Delete("acme"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := store.Get("acme")
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestStore_DeleteNonExistentReturnsError(t *testing.T) {
	_, store, _ := setupStores(t)

	err := store.Delete("ghost")
	if err == nil {
		t.Fatal("expected error deleting non-existent tenant, got nil")
	}
}

func TestStore_GetNonExistentReturnsError(t *testing.T) {
	_, store, _ := setupStores(t)

	_, err := store.Get("nope")
	if err == nil {
		t.Fatal("expected error for non-existent tenant, got nil")
	}
}

func TestStore_AddServiceAndListServices(t *testing.T) {
	_, store, _ := setupStores(t)

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := store.AddService(tenant.ID, "web", "github.com/acme/web"); err != nil {
		t.Fatalf("add service web: %v", err)
	}
	if err := store.AddService(tenant.ID, "api", "github.com/acme/api"); err != nil {
		t.Fatalf("add service api: %v", err)
	}

	services, err := store.ListServices(tenant.ID)
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
	found := map[string]bool{}
	for _, s := range services {
		found[s.ServiceName] = true
		if s.TenantID != tenant.ID {
			t.Errorf("service tenant_id = %d, want %d", s.TenantID, tenant.ID)
		}
	}
	if !found["web"] || !found["api"] {
		t.Fatalf("missing expected services in %v", services)
	}
}

func TestStore_ForeignKeyCascadeDeletesServices(t *testing.T) {
	db, store, _ := setupStores(t)

	// Verify foreign keys are enabled.
	var fkEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
		t.Fatalf("check foreign_keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Fatal("PRAGMA foreign_keys is OFF; cascade will not work")
	}

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.AddService(tenant.ID, "web", "github.com/acme/web"); err != nil {
		t.Fatalf("add service: %v", err)
	}

	// Delete the tenant; services should be cascaded.
	if err := store.Delete("acme"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	services, err := store.ListServices(tenant.ID)
	if err != nil {
		t.Fatalf("list services after delete: %v", err)
	}
	if len(services) != 0 {
		t.Fatalf("expected 0 services after cascade delete, got %d", len(services))
	}
}

// ---------------------------------------------------------------------------
// KeyStore tests
// ---------------------------------------------------------------------------

func TestKeyStore_GenerateHasRWPrefix(t *testing.T) {
	_, store, ks := setupStores(t)

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	rawKey, key, err := ks.Generate(tenant.ID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(rawKey) < 4 || rawKey[:3] != "rw_" {
		t.Fatalf("raw key %q does not start with 'rw_'", rawKey)
	}
	if key.TenantID != tenant.ID {
		t.Fatalf("key tenant_id = %d, want %d", key.TenantID, tenant.ID)
	}
	if key.ID == 0 {
		t.Fatal("expected non-zero key ID")
	}
}

func TestKeyStore_ValidateCorrectKey(t *testing.T) {
	_, store, ks := setupStores(t)

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	rawKey, _, err := ks.Generate(tenant.ID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	tenantID, err := ks.Validate(rawKey)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if tenantID != tenant.ID {
		t.Fatalf("validate returned tenant_id %d, want %d", tenantID, tenant.ID)
	}
}

func TestKeyStore_ValidateWrongKeyReturnsError(t *testing.T) {
	_, store, ks := setupStores(t)

	if _, err := store.Create("acme"); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	_, err := ks.Validate("rw_boguskey1234567890abcdef")
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}
}

func TestKeyStore_ListForTenant(t *testing.T) {
	_, store, ks := setupStores(t)

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	// Generate two keys.
	for i := 0; i < 2; i++ {
		if _, _, err := ks.Generate(tenant.ID); err != nil {
			t.Fatalf("generate %d: %v", i, err)
		}
	}

	keys, err := ks.ListForTenant(tenant.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	for _, k := range keys {
		if k.TenantID != tenant.ID {
			t.Errorf("key tenant_id = %d, want %d", k.TenantID, tenant.ID)
		}
		// Hash should not be populated in ListForTenant (it is not selected).
		if k.Hash != "" {
			t.Errorf("expected empty hash in list, got %q", k.Hash)
		}
	}
}

func TestKeyStore_RevokeRemovesKey(t *testing.T) {
	_, store, ks := setupStores(t)

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	rawKey, key, err := ks.Generate(tenant.ID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if err := ks.Revoke(key.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}

	// Key should no longer validate.
	_, err = ks.Validate(rawKey)
	if err == nil {
		t.Fatal("expected error after revoke, got nil")
	}

	// ListForTenant should return empty.
	keys, err := ks.ListForTenant(tenant.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after revoke, got %d", len(keys))
	}
}

func TestKeyStore_RevokeNonExistentReturnsError(t *testing.T) {
	_, _, ks := setupStores(t)

	err := ks.Revoke(99999)
	if err == nil {
		t.Fatal("expected error revoking non-existent key, got nil")
	}
}

func TestKeyStore_ForeignKeyCascadeDeletesKeys(t *testing.T) {
	db, store, ks := setupStores(t)

	// Verify foreign keys are enabled.
	var fkEnabled int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
		t.Fatalf("check foreign_keys: %v", err)
	}
	if fkEnabled != 1 {
		t.Fatal("PRAGMA foreign_keys is OFF; cascade will not work")
	}

	tenant, err := store.Create("acme")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	rawKey, _, err := ks.Generate(tenant.ID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Delete the tenant; API keys should be cascaded.
	if err := store.Delete("acme"); err != nil {
		t.Fatalf("delete tenant: %v", err)
	}

	// Key should no longer validate.
	_, err = ks.Validate(rawKey)
	if err == nil {
		t.Fatal("expected error after tenant delete, got nil")
	}

	// ListForTenant should return empty.
	keys, err := ks.ListForTenant(tenant.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys after cascade delete, got %d", len(keys))
	}
}
