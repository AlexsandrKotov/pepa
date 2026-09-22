//go:build integration

package bootstrap

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/crypto"
	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/testenv"
)

func TestMigrateConnectionTLSKeys(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", "connection-migration-regression-test-key")
	pg, err := testenv.StartPostgres(t.Context(), t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })
	repo := repository.NewConnectionRepository(pg.DB())
	comp := &Components{DB: pg.DB(), ConnectionRepo: repo}
	conn := &repository.Connection{
		TenantID: uuid.New(), Type: repository.ConnectionDocker, Name: "tls-migration-test",
		Status: "disconnected", FallbackToAdmin: false,
		Config: map[string]any{"tls_key": "new-private-key", "host": "tcp://docker.example.com:2376"},
		Labels: map[string]string{},
	}
	if err := repo.Create(t.Context(), conn); err != nil {
		t.Fatal(err)
	}
	readKey := func() string {
		t.Helper()
		var value string
		if err := pg.DB().Pool.QueryRow(t.Context(), "SELECT config->>'tls_key' FROM connections WHERE id=$1", conn.ID).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if !crypto.IsEncrypted(readKey()) {
		t.Fatal("new connection stored a plaintext key")
	}
	conn.Config["tls_key"] = "updated-private-key"
	if err := repo.Update(t.Context(), conn); err != nil {
		t.Fatal(err)
	}
	if !crypto.IsEncrypted(readKey()) {
		t.Fatal("connection update stored a plaintext key")
	}
	// Simulate a record written before tls_key was classified as sensitive.
	if _, err := pg.DB().Pool.Exec(t.Context(), `UPDATE connections SET config=jsonb_set(config, '{tls_key}', to_jsonb($2::text)) WHERE id=$1`, conn.ID, "legacy-private-key"); err != nil {
		t.Fatal(err)
	}
	comp.migrateConnectionCredentials(t.Context())
	first := readKey()
	if !crypto.IsEncrypted(first) {
		t.Fatal("migration did not encrypt the legacy key")
	}
	decrypted, err := repo.GetDecrypted(t.Context(), conn.ID, conn.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted.Config["tls_key"] != "legacy-private-key" {
		t.Fatal("migration changed the key value")
	}
	if decrypted.FallbackToAdmin {
		t.Fatal("migration changed fallback policy")
	}
	comp.migrateConnectionCredentials(t.Context())
	if readKey() != first {
		t.Fatal("migration is not idempotent")
	}
}
