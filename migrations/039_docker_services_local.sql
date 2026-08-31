-- Allow docker_services to target the local Docker daemon (no registered host).
-- docker_host_id becomes nullable; NULL means "local Docker socket".
-- Add folder_path for deploying from a server-side project directory.

ALTER TABLE docker_services ALTER COLUMN docker_host_id DROP NOT NULL;
ALTER TABLE docker_services ADD COLUMN IF NOT EXISTS folder_path TEXT DEFAULT '';
ALTER TABLE docker_services ALTER COLUMN compose_yaml DROP NOT NULL;
