# Trivy Database Management & VEX Integration

This document describes PEPA's enhanced Trivy vulnerability database management system with multi-source caching and VEX (Vulnerability Exploitability eXchange) support.

## Overview

PEPA now includes a robust Trivy DB management system that:

- **Automatically updates** `trivy-db` and `trivy-java-db` every 6 hours
- **Uses multiple sources** with automatic fallback for resilience
- **Supports VEX documents** for filtering false positives
- **Persists DB cache** across container restarts

## Architecture

### Components

1. **db-cache Service** (`pepa-db-cache`)
   - Lightweight Alpine container with Trivy
   - Runs `trivy-cache-warmer.sh` in a loop
   - Downloads DBs from primary → fallback registries
   - Shares cache volume with api-server

2. **API Server** (`pepa-api`)
   - Uses cached DBs for scans
   - Supports VEX documents for noise reduction
   - Passes `--vex` flag to Trivy when configured

3. **Cache Volume** (`trivy-cache`)
   - Named Docker volume persisted across restarts
   - Contains both `trivy-db` and `trivy-java-db`
   - Shared between db-cache and api-server

### DB Sources (Fallback Chain)

**trivy-db:**
1. `public.ecr.aws/aquasecurity/trivy-db` (primary)
2. `ghcr.io/aquasecurity/trivy-db` (fallback 1)
3. `mirror.gcr.io/aquasecurity/trivy-db` (fallback 2)

**trivy-java-db:**
1. `public.ecr.aws/aquasecurity/trivy-java-db` (primary)
2. `ghcr.io/aquasecurity/trivy-java-db` (fallback 1)

## Configuration

### Environment Variables

Add to your `.env` file (see `.env.example`):

```bash
# Primary DB registries
TRIVY_DB_REPOSITORY=public.ecr.aws/aquasecurity/trivy-db
TRIVY_JAVA_DB_REPOSITORY=public.ecr.aws/aquasecurity/trivy-java-db

# Fallback registries (optional, defaults shown)
TRIVY_DB_FALLBACK_1=ghcr.io/aquasecurity/trivy-db
TRIVY_DB_FALLBACK_2=mirror.gcr.io/aquasecurity/trivy-db
TRIVY_JAVA_DB_FALLBACK_1=ghcr.io/aquasecurity/trivy-java-db

# Update interval (default: 21600 = 6 hours)
TRIVY_DB_UPDATE_INTERVAL=21600

# VEX repository URI (optional)
TRIVY_VEX_REPOSITORY_URI=
```

### VEX Documents

VEX (Vulnerability Exploitability eXchange) documents allow you to filter false positives and mark vulnerabilities as "not affected" or "fixed".

#### Creating VEX Directory

```bash
mkdir -p deployments/compose/trivy-vex
```

#### Placing VEX Files

Put your VEX documents (OpenVEX or CycloneDX format) in:

```
deployments/compose/trivy-vex/
├── my-app-vex.json
├── production-exclusions.json
└── accepted-risks.json
```

#### Using VEX in Scan Configuration

When creating a Trivy scan target, specify the VEX file path:

```json
{
  "target": "my-app:latest",
  "scan_type": "image",
  "severity": "HIGH,CRITICAL",
  "vex": "/etc/trivy/vex/my-app-vex.json"
}
```

The scanner will pass `--vex /etc/trivy/vex/my-app-vex.json` to Trivy, filtering out vulnerabilities marked as non-exploitable.

#### VEX Document Format (OpenVEX)

Example VEX document:

```json
{
  "@context": "https://openvex.dev/ns/v0.2.0",
  "@id": "https://openvex.dev/docs/public/vex-PEPA-001",
  "author": "PEPA Security Team",
  "role": "document-creator",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": 1,
  "statements": [
    {
      "vulnerability": {
        "@id": "CVE-2023-1234"
      },
      "products": [
        {
          "@id": "pkg:docker/my-app@1.2.3"
        }
      ],
      "status": "not_affected",
      "justification": "vulnerable_code_not_in_execute_path",
      "impact_statement": "This CVE affects a code path not used in our deployment"
    }
  ]
}
```

## Deployment

### 1. Build and Start Services

```bash
cd deployments/compose
docker compose build db-cache
docker compose up -d
```

This starts:
- `pepa-db-cache` — downloads and refreshes DBs every 6h
- `pepa-api` — uses cached DBs for scans
- `pepa-worker` — background job processor
- `pepa-frontend` — web UI

### 2. Verify DB Cache Warmer

```bash
docker logs pepa-db-cache
```

Expected output:
```
[2024-01-15 10:00:00] Trivy DB Cache Warmer started
[2024-01-15 10:00:00] Update interval: 21600s
[2024-01-15 10:00:00] Cache directory: /tmp/trivy-cache
[2024-01-15 10:00:00] === Starting DB update cycle ===
[2024-01-15 10:00:00] Starting trivy-db update...
[2024-01-15 10:00:00] Trying primary: public.ecr.aws/aquasecurity/trivy-db
[2024-01-15 10:00:05] ✓ trivy-db downloaded from public.ecr.aws/aquasecurity/trivy-db
[2024-01-15 10:00:05] trivy-db update successful
[2024-01-15 10:00:05] Starting trivy-java-db update...
[2024-01-15 10:00:05] Trying primary: public.ecr.aws/aquasecurity/trivy-java-db
[2024-01-15 10:00:10] ✓ trivy-java-db downloaded from public.ecr.aws/aquasecurity/trivy-java-db
[2024-01-15 10:00:10] trivy-java-db update successful
[2024-01-15 10:00:10] === DB update cycle complete, sleeping for 21600s ===
```

### 3. Run a Trivy Scan

From the PEPA web UI:
1. Navigate to **Pipelines** → **Security Scanners**
2. Click **Add Engine** → **Trivy**
3. Configure scan target (image, repo, filesystem)
4. (Optional) Add VEX file path: `/etc/trivy/vex/my-vex.json`
5. Click **Create**
6. Click **Scan** to run the scan

Or via API:

```bash
curl -X POST http://localhost:8088/api/v1/security-scans/scan \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "target_ref": "nginx:latest",
    "scanner_type": "trivy",
    "scan_config": {
      "scan_type": "image",
      "severity": "HIGH,CRITICAL",
      "vex": "/etc/trivy/vex/production-exclusions.json"
    }
  }'
```

## Monitoring

### Check DB Cache Status

```bash
# View last update time
docker exec pepa-db-cache ls -lh /tmp/trivy-cache/db/

# Check cache size
docker exec pepa-db-cache du -sh /tmp/trivy-cache/
```

### View Scan Logs

```bash
docker logs pepa-api | grep -i trivy
```

### Force DB Update

```bash
docker restart pepa-db-cache
```

## Troubleshooting

### DB Download Fails

**Symptom:** All fallback registries fail

**Solution:**
1. Check network connectivity: `docker exec pepa-db-cache ping public.ecr.aws`
2. Verify DNS resolution: `docker exec pepa-db-cache nslookup public.ecr.aws`
3. Check firewall/proxy rules
4. Try custom mirror in `.env`:
   ```bash
   TRIVY_DB_REPOSITORY=your-internal-mirror.com/trivy-db
   ```

### VEX File Not Found

**Symptom:** Scanner logs "VEX document not found, skipping"

**Solution:**
1. Verify file exists: `ls deployments/compose/trivy-vex/`
2. Check mount in api-server: `docker exec pepa-api ls /etc/trivy/vex/`
3. Ensure file permissions allow read access

### Stale DB

**Symptom:** Scan results show outdated vulnerabilities

**Solution:**
1. Check db-cache logs: `docker logs pepa-db-cache`
2. Force update: `docker restart pepa-db-cache`
3. Verify cache volume: `docker volume inspect pepa_trivy-cache`

## Best Practices

### 1. Use Multiple DB Sources

Configure fallback registries to ensure DB availability even if primary source is down.

### 2. Regular VEX Updates

Maintain VEX documents and update them as your application evolves:
- Mark known false positives
- Document accepted risks
- Track remediation progress

### 3. Monitor DB Freshness

Set up alerts if db-cache hasn't updated in >12 hours:
```bash
docker inspect pepa-db-cache --format='{{.State.StartedAt}}'
```

### 4. Cache Volume Backup

Periodically backup the trivy-cache volume for disaster recovery:
```bash
docker run --rm -v pepa_trivy-cache:/data -v $(pwd):/backup \
  alpine tar czf /backup/trivy-cache-backup.tar.gz /data
```

### 5. Resource Limits

The db-cache service is configured with minimal resources (0.25 CPU, 256MB RAM). Adjust if needed:

```yaml
deploy:
  resources:
    limits:
      cpus: '0.5'
      memory: 512M
```

## Security Considerations

- **VEX documents are trusted**: Only use VEX files from trusted sources
- **DB integrity**: Trivy verifies DB signatures automatically
- **Network isolation**: db-cache runs in `pepa-net` network (internal only)
- **Read-only mounts**: VEX directory mounted read-only in api-server

## References

- [Trivy Documentation](https://aquasecurity.github.io/trivy/)
- [OpenVEX Specification](https://github.com/openvex/spec)
- [CycloneDX VEX](https://cyclonedx.org/capabilities/vex/)
- [Trivy DB Repository](https://github.com/aquasecurity/trivy-db)

## Support

For issues or questions:
1. Check logs: `docker logs pepa-db-cache` and `docker logs pepa-api`
2. Verify configuration in `.env`
3. Review this documentation
4. Open an issue on GitHub
