// Package tenant provides multi-tenancy CRUD and API key management for ReleaseWave.
package tenant

import (
	"database/sql"
	"fmt"
	"time"
)

// Tenant represents an organization or team.
type Tenant struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Plan      string    `json:"plan"` // "free" or "pro"
	CreatedAt time.Time `json:"created_at"`
}

// TenantService links a service to a tenant.
type TenantService struct {
	TenantID    int64  `json:"tenant_id"`
	ServiceName string `json:"service_name"`
	Repo        string `json:"repo"`
}

// Store provides tenant CRUD operations backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a tenant store using the provided database connection.
func NewStore(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS tenants (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			plan TEXT NOT NULL DEFAULT 'free',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tenant_services (
			tenant_id INTEGER NOT NULL,
			service_name TEXT NOT NULL,
			repo TEXT NOT NULL,
			FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
			UNIQUE(tenant_id, service_name)
		)`,
	}
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Create adds a new tenant.
func (s *Store) Create(name, plan string) (*Tenant, error) {
	if plan == "" {
		plan = "free"
	}
	result, err := s.db.Exec(
		`INSERT INTO tenants (name, plan) VALUES (?, ?)`,
		name, plan,
	)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get tenant id: %w", err)
	}
	return &Tenant{ID: id, Name: name, Plan: plan, CreatedAt: time.Now()}, nil
}

// Get retrieves a tenant by name.
func (s *Store) Get(name string) (*Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(
		`SELECT id, name, plan, created_at FROM tenants WHERE name = ?`, name,
	).Scan(&t.ID, &t.Name, &t.Plan, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tenant %q not found", name)
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List returns all tenants.
func (s *Store) List() ([]Tenant, error) {
	rows, err := s.db.Query(`SELECT id, name, plan, created_at FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tenants []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Plan, &t.CreatedAt); err != nil {
			return nil, err
		}
		tenants = append(tenants, t)
	}
	return tenants, rows.Err()
}

// Delete removes a tenant and its services.
func (s *Store) Delete(name string) error {
	result, err := s.db.Exec(`DELETE FROM tenants WHERE name = ?`, name)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("tenant %q not found", name)
	}
	return nil
}

// AddService links a service to a tenant.
func (s *Store) AddService(tenantID int64, serviceName, repo string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO tenant_services (tenant_id, service_name, repo) VALUES (?, ?, ?)`,
		tenantID, serviceName, repo,
	)
	return err
}

// ListServices returns all services for a tenant.
func (s *Store) ListServices(tenantID int64) ([]TenantService, error) {
	rows, err := s.db.Query(
		`SELECT tenant_id, service_name, repo FROM tenant_services WHERE tenant_id = ?`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []TenantService
	for rows.Next() {
		var ts TenantService
		if err := rows.Scan(&ts.TenantID, &ts.ServiceName, &ts.Repo); err != nil {
			return nil, err
		}
		services = append(services, ts)
	}
	return services, rows.Err()
}
