-- 014_gitops_repositories.sql
-- Stores Git repositories that contain GitOps manifests (FluxCD / ArgoCD).
-- Users bind a repo + branch + path; PEPA scans it for HelmRelease, Kustomization,
-- and ArgoCD Application resources.

CREATE TABLE IF NOT EXISTS gitops_repositories (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL,
  name          TEXT NOT NULL,
  connection_id UUID REFERENCES connections(id) ON DELETE SET NULL,
  repo_url      TEXT NOT NULL,
  branch        TEXT NOT NULL DEFAULT 'main',
  path          TEXT NOT NULL DEFAULT '.',
  engine_type   TEXT NOT NULL DEFAULT 'auto',   -- 'fluxcd' | 'argocd' | 'auto'
  scan_status   TEXT NOT NULL DEFAULT 'pending', -- 'pending' | 'scanning' | 'ready' | 'error'
  scan_error    TEXT,
  last_scanned_at TIMESTAMPTZ,
  config        JSONB NOT NULL DEFAULT '{}',    -- token ref, ssh key ref, etc.
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gitops_repos_tenant ON gitops_repositories(tenant_id);
