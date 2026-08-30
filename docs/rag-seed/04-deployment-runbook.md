# PEPA — Deployment Runbook

## Architecture

PEPA runs as 3 Docker containers:
- `pepa-api` — Go backend (port 8088)
- `pepa-frontend` — Next.js frontend (port 3000)
- `pepa-postgres` — PostgreSQL 18 with pgvector (port 5432)
- `pepa-redis` — Redis 7 (port 6379)

Optional: nginx reverse proxy for SSL termination.

## Deploying Updates

### Standard Update (no migration)

```bash
cd deployments/compose
docker compose pull
docker compose up -d --build
```

### Update with Migration

Database migrations run automatically on startup. If a migration fails:

```bash
# Check logs
docker compose logs pepa-api | grep migration

# Rollback (restore from backup)
docker compose down
# Restore database from backup
docker compose up -d
```

## Health Checks

```bash
# Basic health
curl http://localhost:8088/healthz

# Readiness (checks DB + Redis)
curl http://localhost:8088/readyz

# Expected response:
# {"status":"ok","version":"dev","app":"PEPA — Platform Engineering & Pipeline Automator"}
```

## Common Issues

### API returns 503 "bootstrap required"
The system needs first-run setup. Use the bootstrap token:
```bash
curl -X POST http://localhost:8088/api/v1/auth/bootstrap \
  -H "Authorization: Bearer <bootstrap-token>"
```

### PostgreSQL "disk pressure" or connection refused
```bash
# Check disk space
df -h

# Check Postgres logs
docker compose logs pepa-postgres | tail -50

# Restart Postgres
docker compose restart pepa-postgres
```

### Redis connection lost
```bash
# Check Redis
docker compose exec pepa-redis redis-cli ping

# Restart Redis
docker compose restart pepa-redis
```

### Plugin crashes
```bash
# Check which plugin failed
docker compose logs pepa-api | grep "plugin crashed"

# Disable the problematic plugin via API
curl -X PUT http://localhost:8088/api/v1/plugins/<name>/disable \
  -H "Authorization: Bearer <token>"

# Restart API
docker compose restart pepa-api
```

### AI provider errors
```bash
# Check AI configuration
docker compose logs pepa-api | grep "AI"

# Common: API key invalid or rate limited
# Fix: Update AI provider in Connections page
```

## Backup and Restore

### Database Backup
```bash
docker compose exec pepa-postgres pg_dump -U pepa pepa_db > backup_$(date +%Y%m%d).sql
```

### Database Restore
```bash
cat backup_20260830.sql | docker compose exec -T pepa-postgres psql -U pepa pepa_db
```

### Full Data Wipe (Development Only)
```bash
docker compose down -v
docker compose up -d --build
```

## Monitoring

PEPA exposes Prometheus metrics at `GET /metrics`. Key metrics:
- `pepa_http_requests_total` — request rate by path/status
- `pepa_http_request_duration_seconds` — latency histogram
- `pepa_plugin_executions_total` — plugin call count
- `pepa_ai_requests_total` — AI request count by provider

## Scaling

For production HA:
- Run 2+ `pepa-api` instances behind a load balancer
- Use managed PostgreSQL (RDS, Cloud SQL) instead of Docker
- Use managed Redis (ElastiCache, MemoryDB)
- Frontend is stateless — scale horizontally
