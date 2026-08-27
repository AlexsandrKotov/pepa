# Security Policy

## Supported Versions

| Version | Supported | End of Life |
|---------|-----------|-------------|
| 1.0.x   | Yes       | TBD         |

We actively support the latest major release and one prior major release with critical security patches.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

If you discover a security vulnerability in PEPA, please report it responsibly:

### Option 1: GitHub Private Vulnerability Reporting (Preferred)

1. Go to the [Security Advisories page](https://github.com/AlexsandrKotov/pepa/security/advisories/new)
2. Click **Report a vulnerability**
3. Fill in the advisory form with as much detail as possible

### Option 2: Email

Send an email to **security@github.com** with:

> **Note**: If `security@github.com` is not yet configured, use [GitHub Private Security Reporting](https://github.com/AlexsandrKotov/pepa/security/advisories/new) instead.

- Description of the vulnerability
- Steps to reproduce or proof of concept
- Impact assessment
- Suggested fix (if any)

### What to Expect

| Step | Timeline |
|------|----------|
| Acknowledgement of receipt | Within 48 hours |
| Initial triage and severity assessment | Within 7 days |
| Fix development and testing | Varies by severity |
| Patch release | Within 30 days for critical/high |
| Public disclosure | After patch is available |

### Severity Levels

| Level | Description | Response Time |
|-------|-------------|---------------|
| **Critical** | Remote code execution, auth bypass, data breach | Immediate patch |
| **High** | Privilege escalation, sensitive data exposure | Patch within 7 days |
| **Medium** | DoS, information disclosure | Patch within 30 days |
| **Low** | Minor information leak, best practice violation | Next scheduled release |

## Security Architecture

PEPA is designed with security in mind:

- **Authentication**: JWT-based with bootstrap token for initial setup
- **Authorization**: RBAC with row-level security (RLS) in PostgreSQL
- **Secrets**: HashiCorp Vault integration (KV v2) with AES-256 encryption at rest
- **Network**: TLS-ready reverse proxy (Nginx), CORS configurable, rate limiting
- **Plugins**: Process isolation via HashiCorp go-plugin (gRPC), plugin signing
- **Audit**: Immutable audit log for all platform actions

## Security Best Practices for Deployments

1. **Change default credentials** immediately after bootstrap
2. **Enable TLS** for all external-facing endpoints
3. **Use Vault** for secret storage instead of environment variables
4. **Restrict CORS origins** to known domains in production
5. **Set `DEV_MODE=false`** in production environments
6. **Rotate JWT secrets** periodically
7. **Keep dependencies updated** — run `go mod tidy` and `npm audit fix` regularly
8. **Scan Docker images** with Trivy before deploying to production

## Bug Bounty Program

A formal bug bounty program will be announced after the v0.1.0 GA release. In the meantime, we appreciate responsible disclosure of any security issues.

## Security Updates

Security advisories will be published via:

- [GitHub Security Advisories](https://github.com/AlexsandrKotov/pepa/security/advisories)
- Release notes for patch versions
- Community Slack (post-launch)

## Contact

- Security reports: **[GitHub Security Advisory](https://github.com/AlexsandrKotov/pepa/security/advisories/new)** (preferred)
- Email fallback: **security@github.com** (if configured)
- General questions: **GitHub Discussions**
- Urgent matters: GitHub Security Advisory with "Critical" severity
