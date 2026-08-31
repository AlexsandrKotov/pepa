-- Allow docker_services to target the local Docker daemon (no registered host).
-- docker_host_id becomes nullable; NULL means "local Docker socket".

ALTER TABLE docker_services ALTER COLUMN docker_host_id DROP NOT NULL;
