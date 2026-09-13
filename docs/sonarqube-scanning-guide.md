# SonarQube Report Collection

## Overview

PEPA does not analyse code itself. For SonarQube it acts as a **report collector**: the
analysis runs in your CI or on the SonarQube server, and PEPA pulls the latest
results — Quality Gate, issues, measures — over the REST API, stores them as a
`scan_run`, and renders them in the same Security tabs that Trivy uses.

The only configuration PEPA needs is the SonarQube URL and an access token, and
both live in a single **Connection of type `sonarqube`**. No JVM, no
`sonar-scanner` binary, no Docker socket, no repository checkout is required on
the PEPA host.

## Architecture

### How it works

```
┌──────────────┐   analysis    ┌──────────────┐
│ GitLab CI /  │ ────────────▶ │ SonarQube    │
│ GitHub / dev │               │ (external)   │
└──────────────┘               └──────┬───────┘
                                      │ REST API
                                      ▼
                              ┌──────────────┐
                              │ PEPA api     │
                              │ (report      │
                              │  collector)  │
                              └──────┬───────┘
                                      │
                                      ▼
                              ┌──────────────┐
                              │ PostgreSQL   │
                              │ scan_runs    │
                              └──────────────┘
```

Three triggers feed the collector:

1. **Manual** — `Scan Now` in the Security UI.
2. **Schedule** — a `scan_schedule` row on the target (e.g. every 6 hours).
3. **Webhook** — SonarQube Compute Operator webhook fires `POST /api/v1/security/sonar/webhook`
   and PEPA collects a fresh report for every target that owns the project.

### What PEPA does not do

- It does not run `sonar-scanner` as a subprocess.
- It does not clone the source repository.
- It does not install a JVM or a SonarQube server.
- It does not create projects in SonarQube.

If you need a pipeline step that runs the analysis, use the standard
`sonar-scanner` CLI in your CI (GitLab CI, GitHub Actions, Jenkins, …). PEPA
only consumes the result.

## Configuration

### 1. Create a SonarQube Connection

In **Connections → Add Connection → SonarQube**, fill in:

| Field   | Meaning                                                                 |
|---------|-------------------------------------------------------------------------|
| URL     | Base URL of your SonarQube instance (e.g. `https://sonarqube.example.com`). |
| Token   | A SonarQube user token (`sqp_…`). Generate one in SonarQube → *My Account → Security*. |
| Insecure | Skip TLS verification — useful for self-signed certificates on internal instances. |

The token is stored encrypted at rest. Read access to **projects / issues /
measures** is enough to collect reports; marking an issue as false positive
requires permission to edit issues.

### 2. Create a Scan Target

In **Security → Add Target → Code Quality (SonarQube)**:

- **Connection** — pick the SonarQube connection from step 1.
- **Project Key** — the identifier of the project inside SonarQube. The picker
  autocompletes from `GET /api/v1/security/sonar/projects` so you never have to
  type the key blindly.
- **Branch** (optional) — defaults to the project's main branch.
- **Stale after, hours** (optional) — a snapshot older than this is flagged as
  *Stale* in the UI. Defaults to the environment value
  `SONAR_DEFAULT_STALE_HOURS` (24).

### 3. Environment variables

| Variable                    | Default | Meaning                                                                 |
|-----------------------------|---------|-------------------------------------------------------------------------|
| `SONAR_SCAN_TIMEOUT`        | `2m`    | Upper bound for collecting one report (a handful of API calls, not a scan). Accepts a Go duration (`90s`, `2m`) or plain seconds. |
| `SONAR_DEFAULT_STALE_HOURS` | `24`    | Freshness budget for targets that do not configure their own `stale_after_hours`. |
| `SONAR_WEBHOOK_TOKEN`       | *(empty)* | Shared secret for the incoming SonarQube webhook. While unset the endpoint answers `503` and report collection runs on schedules only. |

All three are documented in `.env.example` and in the Helm `values.yaml`
(`apiServer.env` for the non-secret ones, `secrets.sonarWebhookToken` for the
secret).

### `scan_config` keys

The target's `scan_config` JSONB accepts only keys the scanner understands:

| Key                  | Scanner   | Meaning                                                                 |
|----------------------|-----------|-------------------------------------------------------------------------|
| `project_key`        | sonarqube | Overrides `target_ref` as the SonarQube project key.                    |
| `branch`             | sonarqube | Branch to report on (defaults to the project's main branch).            |
| `severity`           | sonarqube | Minimum severity to include (`INFO`, `MINOR`, `MAJOR`, `CRITICAL`, `BLOCKER`). |
| `stale_after_hours`  | sonarqube | Per-target freshness budget.                                            |
| `source_ci_url`      | sonarqube | Optional link to the CI pipeline that produced the analysis.            |

`url` and `token` are **rejected** — credentials live in the Connection, and
`scan_config` is stored as plain JSONB.

## API Reference

### List SonarQube projects (for the picker)

```
GET /api/v1/security/sonar/projects?connection_id=<uuid>
```

Returns the projects the token can see, up to 500. The UI uses this to
autocomplete the Project Key field.

### Trigger a report collection

```
POST /api/v1/security/targets/:id/scan
```

Identical to the Trivy path: the same endpoint triggers the same `scan_run`
lifecycle. The scanner type is taken from the target.

### Server-side report

```
GET /api/v1/security/scans/:id/report?format=json|html
```

Renders the stored run as a JSON or HTML report. Both scanners share one code
path, so a SonarQube run exports exactly like a Trivy one.

### Transition an issue upstream

```
POST /api/v1/security/sonar/issues/transition
{
  "connection_id": "<uuid>",
  "issue_key":     "AYx…",
  "transition":    "falsepositive"   // or "wontfix", "reopen", "accept", "confirm"
}
```

PEPA proxies the call to SonarQube and stores nothing: the next collected
report reflects the new upstream state.

### Webhook endpoint (public)

```
POST /api/v1/security/sonar/webhook
Headers: X-PEPA-Signature: <SONAR_WEBHOOK_TOKEN>
```

Receives SonarQube Compute Operator `REPORT` notifications. The endpoint is
**disabled (`503`) while `SONAR_WEBHOOK_TOKEN` is unset** — forgetting to
configure the secret is the mistake we want to surface, not silently accept.

Payload (only the fields PEPA reads):

```json
{
  "type": "REPORT",
  "project": {
    "name": "checkout-service",
    "key":  "checkout-service",
    "url":  "https://sonarqube.example.com/dashboard?id=checkout-service"
  },
  "qualityGate": { "status": "OK" }
}
```

PEPA matches the payload URL against every SonarQube Connection (scheme, host
and path boundary — not a raw string prefix), finds every target that owns the
project, and fires a `RunScan` for each. The response echoes the list of
triggered target IDs.

## Deployment

### Standard deployment

No change to the images: `Dockerfile.api` does not install a JVM or
`sonar-scanner`. The only addition is the three environment variables above.

### Configure the webhook in SonarQube

In SonarQube → *Project Settings → Webhooks* (or *Administration → Configuration → Webhooks* for global):

| Field   | Value                                                              |
|---------|--------------------------------------------------------------------|
| Name    | `PEPA`                                                             |
| URL     | `https://pepa.example.com/api/v1/security/sonar/webhook`           |
| Secret  | The same value as `SONAR_WEBHOOK_TOKEN`                            |

SonarQube signs the request with the secret; PEPA compares it against the
environment variable using constant-time comparison.

### Helm

```yaml
# values.yaml
apiServer:
  env:
    SONAR_SCAN_TIMEOUT: "2m"
    SONAR_DEFAULT_STALE_HOURS: "24"
secrets:
  sonarWebhookToken: "s3cret"
```

The chart renders `SONAR_WEBHOOK_TOKEN` into the `pepa-secrets` Secret, which
the api-server pod already mounts via `envFrom`.

## Report format

PEPA normalises SonarQube output onto the same shape Trivy uses, so the
Security tabs render without branching:

| SonarQube severity | PEPA severity |
|--------------------|---------------|
| `BLOCKER`          | `CRITICAL`    |
| `CRITICAL`         | `HIGH`        |
| `MAJOR`            | `MEDIUM`      |
| `MINOR`            | `LOW`         |
| `INFO`             | `UNKNOWN`     |

The original SonarQube severity is preserved on every finding as
`SonarSeverity`, so the detail panel can show both.

Issues are grouped by component path, which produces the same collapsible
per-container list Trivy users are used to. The summary carries:

- `quality_gate_status`, `bugs`, `vulnerabilities`, `code_smells`, `coverage`,
  `duplicated_lines_density`, `technical_debt`
- `project_key`, `branch`, `last_analysis_at`, `truncated`

When a SonarQube endpoint degrades (e.g. `measures/component` returns 404 but
the Quality Gate is alive), the partial data is kept in
`result_full.sonar_warnings` and the scan still completes successfully.

## Limits

- **5 000 issues per report.** The plugin pages up to that ceiling and sets
  `truncated = true` when more exist. Raise the limit by tuning
  `issuesMaxFetch` in the plugin — the cost is one extra API round-trip per 500
  issues.
- **500 projects in the picker.** Same trade-off.
- **2 minute timeout per report** (default `SONAR_SCAN_TIMEOUT`). A report is a
  handful of API calls, not a scan; anything slower is worth investigating.
- **One tenant per webhook trigger.** Webhooks arrive without an auth context,
  so they resolve inside `database.DefaultTenantID`. Multi-tenant deployments
  should use schedules instead.

## Troubleshooting

### "sonarqube URL not configured"

The target has no Connection, or the Connection it references was deleted.
Open the target, pick a SonarQube Connection, save.

### "sonarqube plugin not configured: url and token are required"

The scanner tried to call the plugin without credentials. This happens when
the Connection cannot be resolved (wrong tenant, deleted, or the encryption
key changed). The error is written to `scan_runs.error_message`, not to a
silent empty report.

### Webhook answers 503

`SONAR_WEBHOOK_TOKEN` is not set on the api-server. Either set it, or use a
schedule instead.

### Webhook answers 401

The `X-PEPA-Signature` header does not match `SONAR_WEBHOOK_TOKEN`. Check that
the same secret is configured on both sides and that SonarQube is not
trimming whitespace when you paste it.

### "project not found" vs "project has not been analyzed"

The first means the project key is wrong (or the token cannot see it). The
second means the project exists but no analysis has run yet — trigger one in
your CI and try again. PEPA distinguishes the two via
`/api/analysis_reports/has_been_analyzed`.

### Report looks empty but the target is green

The target's `last_analysis_at` is older than `stale_after_hours`. The UI
flags it as *Stale* — click **Scan Now** to collect a fresh snapshot.

### Self-signed certificate

Tick **Insecure** on the Connection, or set `insecure: "true"` in the
connection config. The same flag is honoured by the Test button and by the
scan.

## Why PEPA does not run `sonar-scanner`

`sonar-scanner` is a JVM process that needs a checkout of the source code and
a SonarQube server to talk to. Running it inside PEPA would mean:

- Bundling a JRE and the scanner binary in the PEPA image.
- Mounting the Docker socket or cloning repositories into the api-server.
- Duplicating the analysis step your CI already runs.

PEPA's job is to **aggregate** the results of tools that already exist. For
vulnerabilities it is Trivy; for code quality it is SonarQube. The report
collector is the same shape for both: target → fetch → normalise → store →
render.

If you need a CI template that runs the analysis and then notifies PEPA:

```yaml
# .gitlab-ci.yml
sonarqube:
  image: sonarsource/sonar-scanner-cli:latest
  script:
    - sonar-scanner -Dsonar.host.url=$SONAR_HOST_URL -Dsonar.token=$SONAR_TOKEN
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
```

With the webhook configured, SonarQube notifies PEPA at the end of the
analysis and a fresh report appears in the Security tab within seconds.
