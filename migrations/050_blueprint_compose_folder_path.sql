-- ============================================================
-- Migration 050: Blueprint Compose Folder Path & Git URL
-- ============================================================
-- Adds support for specifying a local server folder path or a
-- Git repository URL as the compose source for blueprints,
-- matching the Deploy flow which supports paste-YAML,
-- local-folder, and git-repo sources.
-- ============================================================

ALTER TABLE service_blueprints ADD COLUMN IF NOT EXISTS compose_folder_path TEXT DEFAULT '';
ALTER TABLE service_blueprints ADD COLUMN IF NOT EXISTS compose_git_url TEXT DEFAULT '';
