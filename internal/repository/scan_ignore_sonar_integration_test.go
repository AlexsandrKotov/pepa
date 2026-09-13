//go:build integration

package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/testenv"
)

// SonarQube findings have no CVE, so an ignore row carries issue_key instead.
// Before migration 077 the only unique constraint was (target_id, cve_id), which
// made a second SonarQube ignore on the same target collide on the empty CVE.
func TestScanIgnores_SonarIssueKeyAlongsideCVE(t *testing.T) {
	env := testenv.NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()
	db := env.DB()
	scanRepo := NewSecurityScanRepository(db)
	ignoreRepo := NewScanIgnoreRepository(db)

	tenantA := uuid.New()
	target := &ScanTarget{
		TenantID:    tenantA,
		Name:        "checkout quality",
		ScannerType: "sonarqube",
		TargetType:  "sonarqube_project",
		TargetRef:   "checkout-service",
		ScanConfig:  map[string]any{"project_key": "checkout-service"},
		Enabled:     true,
	}
	if err := scanRepo.CreateScanTarget(ctx, target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	issueKey := "AY8xQ1"
	sonarIgnore := &ScanIgnore{ID: uuid.New(), TenantID: tenantA, TargetID: target.ID, IssueKey: &issueKey}
	if err := ignoreRepo.Create(ctx, sonarIgnore); err != nil {
		t.Fatalf("create sonar ignore: %v", err)
	}
	// A rule-level suppression is just another issue_key value.
	ruleKey := "rule:java:S1135"
	if err := ignoreRepo.Create(ctx, &ScanIgnore{ID: uuid.New(), TenantID: tenantA, TargetID: target.ID, IssueKey: &ruleKey}); err != nil {
		t.Fatalf("create rule ignore: %v", err)
	}
	cve := "CVE-2024-0001"
	if err := ignoreRepo.Create(ctx, &ScanIgnore{ID: uuid.New(), TenantID: tenantA, TargetID: target.ID, CveID: cve}); err != nil {
		t.Fatalf("a Trivy ignore must coexist with SonarQube ones: %v", err)
	}

	ignores, err := ignoreRepo.ListByTarget(ctx, target.ID, tenantA)
	if err != nil {
		t.Fatalf("list ignores: %v", err)
	}
	if len(ignores) != 3 {
		t.Fatalf("expected 3 ignores, got %d: %+v", len(ignores), ignores)
	}
	byIssue := map[string]bool{}
	for _, ig := range ignores {
		if ig.IssueKey != nil {
			byIssue[*ig.IssueKey] = true
		}
	}
	if !byIssue[issueKey] || !byIssue[ruleKey] {
		t.Errorf("issue keys lost: %v", byIssue)
	}

	// The .trivyignore export belongs to Trivy only: a SonarQube key written there
	// would be a silently meaningless line.
	content, err := ignoreRepo.GetIgnoreFileContent(ctx, target.ID, tenantA)
	if err != nil {
		t.Fatalf("ignore file content: %v", err)
	}
	if !strings.Contains(content, cve) {
		t.Errorf("the CVE must be exported, got %q", content)
	}
	if strings.Contains(content, issueKey) || strings.Contains(content, "java:S1135") {
		t.Errorf("SonarQube keys must not leak into the Trivy ignore file: %q", content)
	}

	// Unignore by issue key removes exactly that row.
	if err := ignoreRepo.DeleteByTargetAndIssueKey(ctx, target.ID, issueKey, tenantA); err != nil {
		t.Fatalf("delete by issue key: %v", err)
	}
	ignores, err = ignoreRepo.ListByTarget(ctx, target.ID, tenantA)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(ignores) != 2 {
		t.Fatalf("expected 2 remaining ignores, got %d", len(ignores))
	}
	for _, ig := range ignores {
		if ig.IssueKey != nil && *ig.IssueKey == issueKey {
			t.Error("the deleted issue key is still present")
		}
		if ig.CveID == "" && ig.IssueKey == nil {
			t.Error("an ignore must carry exactly one identifier")
		}
	}
}

// The unique indexes are per target, so two tenants may suppress the same issue
// key, and neither may see the other's list.
func TestScanIgnores_TenantIsolation(t *testing.T) {
	env := testenv.NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()
	db := env.DB()
	scanRepo := NewSecurityScanRepository(db)
	ignoreRepo := NewScanIgnoreRepository(db)

	tenantA, tenantB := uuid.New(), uuid.New()
	key := "shared-issue-key"

	targets := map[uuid.UUID]*ScanTarget{}
	for _, tenant := range []uuid.UUID{tenantA, tenantB} {
		target := &ScanTarget{
			TenantID:    tenant,
			Name:        "target-" + tenant.String()[:8],
			ScannerType: "sonarqube",
			TargetType:  "sonarqube_project",
			TargetRef:   "checkout-service",
			Enabled:     true,
		}
		if err := scanRepo.CreateScanTarget(ctx, target); err != nil {
			t.Fatalf("create target: %v", err)
		}
		targets[tenant] = target
	}

	for _, tenant := range []uuid.UUID{tenantA, tenantB} {
		ignore := &ScanIgnore{ID: uuid.New(), TenantID: tenant, TargetID: targets[tenant].ID, IssueKey: &key}
		if err := ignoreRepo.Create(ctx, ignore); err != nil {
			t.Fatalf("tenant %s could not ignore the same key: %v", tenant, err)
		}
	}

	listA, err := ignoreRepo.List(ctx, tenantA)
	if err != nil {
		t.Fatalf("list tenant A: %v", err)
	}
	if len(listA) != 1 || listA[0].TenantID != tenantA {
		t.Fatalf("tenant A must see exactly its own ignore, got %+v", listA)
	}

	// A wrong tenant must not be able to read or delete the other's ignore.
	if _, err := ignoreRepo.ListByTarget(ctx, targets[tenantB].ID, tenantA); err != nil {
		t.Fatalf("cross-tenant read should return an empty list, not an error: %v", err)
	}
	if err := ignoreRepo.Delete(ctx, listA[0].ID, tenantB); err != nil {
		t.Fatalf("cross-tenant delete: %v", err)
	}
	if remaining, err := ignoreRepo.List(ctx, tenantA); err != nil || len(remaining) != 1 {
		t.Errorf("a delete by another tenant must remove nothing: %d rows, err %v", len(remaining), err)
	}
}

// Duplicate suppression of the same finding on the same target is a no-op that
// used to create two rows; the partial unique index now rejects it.
func TestScanIgnores_DuplicateIssueKeyRejected(t *testing.T) {
	env := testenv.NewTestEnv(t)
	defer env.Cleanup(t)

	ctx := context.Background()
	ignoreRepo := NewScanIgnoreRepository(env.DB())

	tenant := uuid.New()
	target := &ScanTarget{
		TenantID: tenant, Name: "dup-target", ScannerType: "sonarqube",
		TargetType: "sonarqube_project", TargetRef: "checkout-service", Enabled: true,
	}
	if err := NewSecurityScanRepository(env.DB()).CreateScanTarget(ctx, target); err != nil {
		t.Fatalf("create target: %v", err)
	}

	key := "AKK4"
	first := &ScanIgnore{ID: uuid.New(), TenantID: tenant, TargetID: target.ID, IssueKey: &key}
	if err := ignoreRepo.Create(ctx, first); err != nil {
		t.Fatalf("create first ignore: %v", err)
	}
	second := &ScanIgnore{ID: uuid.New(), TenantID: tenant, TargetID: target.ID, IssueKey: &key}
	if err := ignoreRepo.Create(ctx, second); err == nil {
		t.Fatal("the same issue key must not be ignored twice on one target")
	}
}
