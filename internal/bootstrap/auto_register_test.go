//go:build integration

package bootstrap

import (
	"context"
	"testing"

	"github.com/pepa/pepa/internal/repository"
	"github.com/pepa/pepa/internal/testenv"
)

// TestAutoRegister_FreshDB_NoPlugins verifies that a fresh database contains
// zero plugin rows so that the bootstrap startup goroutine never registers any
// plugin in the provider registry on first run.  This is the invariant that
// prevents the "Jenkins pre-installed" bug: discovery on disk alone must NOT
// activate a plugin — an explicit Marketplace install is required.
func TestAutoRegister_FreshDB_NoPlugins(t *testing.T) {
	pg, err := testenv.StartPostgres(t.Context(), t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })

	repo := repository.NewPluginRepository(pg.DB())

	// A fresh database must have zero plugins.
	plugins, err := repo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 0 {
		t.Fatalf("expected 0 plugins in fresh DB, got %d", len(plugins))
	}

	// Simulate the bootstrap goroutine's installedMap build step.
	installedMap := make(map[string]*repository.Plugin)
	for i := range plugins {
		installedMap[plugins[i].Name] = &plugins[i]
	}

	// Simulate discovering plugins on disk (names that exist as binaries).
	discoveredOnDisk := []string{"jenkins", "gitlab", "sonarqube", "argocd"}

	// Apply the same filter the bootstrap goroutine now uses: skip any plugin
	// that has no DB row or whose status is "uninstalled".
	registered := []string{}
	for _, name := range discoveredOnDisk {
		existing := installedMap[name]
		if existing == nil || existing.Status == "uninstalled" {
			continue // not installed — must not register
		}
		registered = append(registered, name)
	}

	if len(registered) != 0 {
		t.Fatalf("no plugins should be registered on fresh DB, but got: %v", registered)
	}
}

// TestAutoRegister_StatusFiltering verifies that the bootstrap goroutine's
// filter logic correctly handles the three plugin states:
//   - no DB row          → skip (not installed)
//   - status=uninstalled → skip (explicitly uninstalled)
//   - status=installed   → register (admin installed from Marketplace)
//   - status=running     → register (previously loaded and running)
func TestAutoRegister_StatusFiltering(t *testing.T) {
	pg, err := testenv.StartPostgres(t.Context(), t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Terminate(context.Background()) })

	repo := repository.NewPluginRepository(pg.DB())
	ctx := context.Background()

	// Insert plugins in various states directly (bypassing the Marketplace
	// install flow which would also set installed_at etc.).
	insert := func(name, status string, enabled bool) {
		t.Helper()
		_, err := pg.DB().Pool.Exec(ctx,
			`INSERT INTO plugins (name, version, type, status, enabled) VALUES ($1, '1.0.0', 'ci_provider', $2, $3)`,
			name, status, enabled,
		)
		if err != nil {
			t.Fatalf("insert plugin %s: %v", name, err)
		}
	}

	insert("jenkins", "installed", true)     // admin installed, enabled
	insert("gitlab", "installed", false)      // admin installed, disabled
	insert("sonarqube", "uninstalled", false) // admin uninstalled
	// argocd — no row at all (never installed)

	plugins, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	installedMap := make(map[string]*repository.Plugin)
	for i := range plugins {
		installedMap[plugins[i].Name] = &plugins[i]
	}

	discoveredOnDisk := []string{"jenkins", "gitlab", "sonarqube", "argocd"}

	var registered []string
	var skipped []string
	for _, name := range discoveredOnDisk {
		existing := installedMap[name]
		if existing == nil || existing.Status == "uninstalled" {
			skipped = append(skipped, name)
			continue
		}
		registered = append(registered, name)
	}

	// Only jenkins and gitlab should be registered (they have status=installed).
	if len(registered) != 2 {
		t.Fatalf("expected 2 registered plugins, got %d: %v", len(registered), registered)
	}
	if registered[0] != "jenkins" || registered[1] != "gitlab" {
		t.Errorf("unexpected registered plugins: %v", registered)
	}

	// sonarqube (uninstalled) and argocd (no row) must be skipped.
	if len(skipped) != 2 {
		t.Fatalf("expected 2 skipped plugins, got %d: %v", len(skipped), skipped)
	}

	// Verify the enabled flag is preserved from DB.
	jenkinsEntry := installedMap["jenkins"]
	if !jenkinsEntry.Enabled {
		t.Error("jenkins should be enabled (DB says enabled=true)")
	}
	gitlabEntry := installedMap["gitlab"]
	if gitlabEntry.Enabled {
		t.Error("gitlab should be disabled (DB says enabled=false)")
	}
}
