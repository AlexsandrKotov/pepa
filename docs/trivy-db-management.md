# Trivy Database Management & VEX Integration

This document describes PEPA's Trivy vulnerability database management system with automatic downloads, multi-source fallback, configurable registries, and VEX support.

## Overview

PEPA includes a robust Trivy DB management system that:

- **Automatically downloads** `trivy-db` and `trivy-java-db` when the Trivy plugin is installed
- **Refreshes databases** in the background every 6 hours (configurable)
- **Uses multiple sources** with automatic fallback for resilience
- **Supports custom DB registries** (ECR, GHCR, private mirrors)
- **Supports VEX documents** for filtering false positives
- **Persists DB cache** across container restarts via shared volume

## Architecture

### How It Works

When the Trivy plugin is installed via the Marketplace (or enabled via the Plugins page), the **API server itself** acts as the DB cache warmer. It downloads both databases into the shared `trivy-cache` volume and starts a background refresh loop.

```
┌──────────────────────────────────────────────────────────────┐
│                     PEPA Platform                             │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  API Server (pepa-api)                                  │  │
│  │                                                          │  │
│  │  ┌──────────────────────────────────────────────────┐  │  │
│  │  │  Scanner (scanner.go)                             │  │  │
│  │  │                                                    │  │  │
│  │  │  • DownloadDB() — downloads trivy-db + java-db    │  │  │
│  │  │  • StartDBManager() — background refresh every 6h │  │  │
│  │  │  • SetDBRepository() — change source at runtime   │  │  │
│  │  │  • Fallback: ECR → GHCR                           │  │  │
│  │  └──────────────────────────────────────────────────┘  │  │
│  │                         │                                │  │
│  │                         │ Shared Volume: trivy-cache     │  │
│  │                         │ /tmp/trivy-cache/              │  │
│  │                         │   ├── db/trivy.db              │  │
│  │                         │   └── java-db/trivy-java.db    │  │
│  │                         │                                │  │
│  │  ┌──────────────────────────────────────────────────┐  │  │
│  │  │  Security Scanner                                 │  │  │
│  │  │  • Reads cached DBs (no re-download)              │  │  │
│  │  │  • Supports VEX filtering                         │  │  │
│  │  │  • Scans: image, fs, repo, config                 │  │  │
│  │  └──────────────────────────────────────────────────┘  │  │
│  └────────────────────────────────────────────────────────┘  │
│                                                               │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  db-cache Service (optional, for standalone setups)     │  │
│  │  • Runs trivy-cache-warmer.sh in a loop                 │  │
│  │  • Kept as fallback for non-standard deployments        │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

### Automatic DB Lifecycle

| Event | Action |
|-------|--------|
| Trivy plugin installed (Marketplace) | DB download starts immediately + background refresh loop |
| Trivy plugin enabled (Plugins page) | DB download starts + background refresh loop |
| API server restarts | Background refresh continues from env config |
| Every 6 hours (default) | Background refresh re-downloads fresh DBs |
| Scan target has `db_repository` in config | Scanner uses that repository for the scan |

### DB Sources (Fallback Chain)

**trivy-db:**
1. `public.ecr.aws/aquasecurity/trivy-db` (primary, configurable)
2. `ghcr.io/aquasecurity/trivy-db` (automatic fallback)

**trivy-java-db:**
1. `public.ecr.aws/aquasecurity/trivy-java-db` (primary, configurable)
2. `ghcr.io/aquasecurity/trivy-java-db` (automatic fallback)

## Configuration

### Environment Variables

Add to your `.env` file (see `.env.example`):

```bash
# Primary DB registries (OCI registries)
TRIVY_DB_REPOSITORY=public.ecr.aws/aquasecurity/trivy-db
TRIVY_JAVA_DB_REPOSITORY=public.ecr.aws/aquasecurity/trivy-java-db

# Fallback registries (optional, defaults shown)
TRIVY_DB_FALLBACK_1=ghcr.io/aquasecurity/trivy-db
TRIVY_DB_FALLBACK_2=mirror.gcr.io/aquasecurity/trivy-db
TRIVY_JAVA_DB_FALLBACK_1=ghcr.io/aquasecurity/trivy-java-db

# DB refresh interval in seconds (default: 21600 = 6 hours)
TRIVY_DB_UPDATE_INTERVAL=21600

# VEX repository URI (optional)
TRIVY_VEX_REPOSITORY_URI=
```

### Plugin Config Schema

The Trivy plugin config supports DB repository fields:

```yaml
config_schema:
  properties:
    db_repository:
      type: string
      description: "OCI registry for the Trivy vulnerability database"
      default: "public.ecr.aws/aquasecurity/trivy-db"
    java_db_repository:
      type: string
      description: "OCI registry for the Trivy Java vulnerability database"
      default: "public.ecr.aws/aquasecurity/trivy-java-db"
```

### Per-Scan-Target DB Repository

Individual scan targets can override the DB repository via `scan_config`:

```json
{
  "target_ref": "my-app:latest",
  "scanner_type": "trivy",
  "scan_config": {
    "scan_type": "image",
    "severity": "HIGH,CRITICAL",
    "db_repository": "ghcr.io/aquasecurity/trivy-db"
  }
}
```

## API Reference

### Check DB Status

```bash
GET /api/v1/security/db-status
```

Returns the availability, update time, and size of both databases.

```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/v1/security/db-status
```

Response:
```json
{
  "trivy_db": {
    "available": true,
    "updated_at": "2024-01-15T10:00:00Z",
    "size_bytes": 1073741824
  },
  "java_db": {
    "available": true,
    "updated_at": "2024-01-15T10:00:00Z",
    "size_bytes": 536870912
  },
  "trivy_version": "Version: 0.74.0"
}
```

### Download DB Manually

```bash
POST /api/v1/security/db-download
```

Triggers an immediate download of both databases. Optionally accepts custom repositories:

```bash
# Download with default repositories
curl -X POST http://localhost:8088/api/v1/security/db-download \
  -H "Authorization: Bearer $TOKEN"

# Download with custom repositories
curl -X POST http://localhost:8088/api/v1/security/db-download \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "db_repository": "ghcr.io/aquasecurity/trivy-db",
    "java_db_repository": "ghcr.io/aquasecurity/trivy-java-db"
  }'
```

### Change DB Repository

```bash
PUT /api/v1/security/db-repository
```

Updates the OCI registry endpoints used for all subsequent DB downloads. No restart required.

```bash
curl -X PUT http://localhost:8088/api/v1/security/db-repository \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "db_repository": "ghcr.io/aquasecurity/trivy-db",
    "java_db_repository": "ghcr.io/aquasecurity/trivy-java-db"
  }'
```

### Install Trivy Plugin with Custom DB

```bash
POST /api/v1/marketplace/trivy/install
```

```bash
# Install with default DB repositories
curl -X POST http://localhost:8088/api/v1/marketplace/trivy/install \
  -H "Authorization: Bearer $TOKEN"

# Install with custom DB mirror
curl -X POST http://localhost:8088/api/v1/marketplace/trivy/install \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "db_repository": "my-mirror.internal.com/trivy-db",
    "java_db_repository": "my-mirror.internal.com/trivy-java-db"
  }'
```

## Deployment

### Standard Deployment

The API server handles DB downloads automatically. No additional setup needed:

1. Deploy PEPA normally: `docker compose up -d`
2. Install the Trivy plugin from the Marketplace
3. Databases are downloaded automatically
4. Check status: `GET /api/v1/security/db-status`

### With db-cache Service (Optional)

For standalone deployments or if you prefer a separate DB cache container:

```bash
cd deployments/compose
docker compose build db-cache
docker compose up -d db-cache
```

The `db-cache` container runs `trivy-cache-warmer.sh` which downloads and refreshes DBs every 6 hours with multi-source fallback.

### Verify DB Status

```bash
# Via API (recommended)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/v1/security/db-status

# Via Docker (if using db-cache container)
docker logs pepa-db-cache

# Check cache files
docker exec pepa-api ls -lh /tmp/trivy-cache/db/
docker exec pepa-api ls -lh /tmp/trivy-cache/java-db/
```

## VEX Documents

VEX (Vulnerability Exploitability eXchange) documents allow you to filter false positives and mark vulnerabilities as "not affected" or "fixed".

### Creating VEX Directory

```bash
mkdir -p deployments/compose/trivy-vex
```

### Placing VEX Files

Put your VEX documents (OpenVEX or CycloneDX format) in:

```
deployments/compose/trivy-vex/
├── my-app-vex.json
├── production-exclusions.json
└── accepted-risks.json
```

### Using VEX in Scan Configuration

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

### VEX Document Format (OpenVEX)

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

## Monitoring

### Check DB Cache Status

```bash
# Via API (recommended)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8088/api/v1/security/db-status

# Via Docker
docker exec pepa-api ls -lh /tmp/trivy-cache/db/
docker exec pepa-api du -sh /tmp/trivy-cache/
```

### View Scan Logs

```bash
docker logs pepa-api | grep -i trivy
```

### Force DB Update

```bash
# Via API (recommended)
curl -X POST http://localhost:8088/api/v1/security/db-download \
  -H "Authorization: Bearer $TOKEN"

# Via Docker (if using db-cache container)
docker restart pepa-db-cache
```

## Troubleshooting

### DB Shows "Not available"

**Symptom:** `GET /api/v1/security/db-status` shows `available: false`

**Solution:**
1. Ensure the Trivy plugin is installed: `POST /api/v1/marketplace/trivy/install`
2. Or trigger a manual download: `POST /api/v1/security/db-download`
3. Check API server logs: `docker logs pepa-api | grep -i trivy`
4. Verify network connectivity: `docker exec pepa-api wget -qO- https://public.ecr.aws`

### DB Download Fails

**Symptom:** All fallback registries fail

**Solution:**
1. Check network connectivity: `docker exec pepa-api nslookup public.ecr.aws`
2. Check firewall/proxy rules
3. Try a different registry:
   ```bash
   curl -X PUT http://localhost:8088/api/v1/security/db-repository \
     -H "Authorization: Bearer $TOKEN" \
     -d '{"db_repository": "ghcr.io/aquasecurity/trivy-db"}'
   ```
4. Or set in `.env`: `TRIVY_DB_REPOSITORY=ghcr.io/aquasecurity/trivy-db`

### VEX File Not Found

**Symptom:** Scanner logs "VEX document not found, skipping"

**Solution:**
1. Verify file exists: `ls deployments/compose/trivy-vex/`
2. Check mount in api-server: `docker exec pepa-api ls /etc/trivy/vex/`
3. Ensure file permissions allow read access

### Stale DB

**Symptom:** Scan results show outdated vulnerabilities

**Solution:**
1. Check DB status: `GET /api/v1/security/db-status`
2. Force refresh: `POST /api/v1/security/db-download`
3. Check API server logs for refresh errors: `docker logs pepa-api | grep "DB refresh"`

## Using Custom DB Mirrors

### Private OCI Registry

If you have a private OCI registry (e.g., AWS ECR, Harbor, GitLab Container Registry):

```bash
# .env
TRIVY_DB_REPOSITORY=registry.internal.com/trivy-db
TRIVY_JAVA_DB_REPOSITORY=registry.internal.com/trivy-java-db
```

### Syncing Trivy DB to Your Registry

Use `skopeo` or `crane` to mirror the Trivy DB to your private registry:

```bash
# Using skopeo
skopeo copy \
  docker://public.ecr.aws/aquasecurity/trivy-db:latest \
  docker://registry.internal.com/trivy-db:latest

# Using crane
crane copy \
  public.ecr.aws/aquasecurity/trivy-db:latest \
  registry.internal.com/trivy-db:latest
```

### About avd.aquasec.com

`https://avd.aquasec.com` is the **web interface** for the Aqua Vulnerability Database. It is not an OCI registry and cannot be used directly as a `--db-repository` value. To use Aqua's databases, use one of the OCI registries:

- `public.ecr.aws/aquasecurity/trivy-db` (AWS, primary)
- `ghcr.io/aquasecurity/trivy-db` (GitHub, fallback)

## Best Practices

### 1. Use the Automatic DB Manager

Install the Trivy plugin from the Marketplace — DBs are downloaded automatically. No manual setup needed.

### 2. Configure Fallback Registries

Set fallback registries in `.env` to ensure DB availability even if the primary source is down.

### 3. Regular VEX Updates

Maintain VEX documents and update them as your application evolves:
- Mark known false positives
- Document accepted risks
- Track remediation progress

### 4. Monitor DB Freshness

Check DB status periodically via the API:
```bash
curl -s http://localhost:8088/api/v1/security/db-status | jq '.trivy_db.updated_at'
```

### 5. Cache Volume Backup

Periodically backup the trivy-cache volume for disaster recovery:
```bash
docker run --rm -v pepa_trivy-cache:/data -v $(pwd):/backup \
  alpine tar czf /backup/trivy-cache-backup.tar.gz /data
```

## Security Considerations

- **VEX documents are trusted**: Only use VEX files from trusted sources
- **DB integrity**: Trivy verifies DB signatures automatically
- **Network isolation**: API server runs in `pepa-net` network (internal only)
- **Read-only mounts**: VEX directory mounted read-only in api-server
- **DB repository validation**: Only OCI registries are accepted as DB sources

## References

- [Trivy Documentation](https://aquasecurity.github.io/trivy/)
- [OpenVEX Specification](https://github.com/openvex/spec)
- [CycloneDX VEX](https://cyclonedx.org/capabilities/vex/)
- [Trivy DB Repository](https://github.com/aquasecurity/trivy-db)
- [Aqua Vulnerability Database (AVD)](https://avd.aquasec.com/)

## Support

For issues or questions:
1. Check DB status: `GET /api/v1/security/db-status`
2. Check logs: `docker logs pepa-api | grep trivy`
3. Verify configuration in `.env`
4. Review this documentation
5. Open an issue on GitHub
