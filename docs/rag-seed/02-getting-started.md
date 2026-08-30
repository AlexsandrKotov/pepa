# PEPA — Getting Started Guide

## Quick Start (Docker Compose)

```bash
git clone https://github.com/AlexsandrKotov/pepa.git
cd pepa/pepa
make docker-up
```

Access the portal at **http://localhost:3000**
Default admin credentials: `admin@example.com` / `admin`

> **Important**: Change the default password after first login.

## Environment Variables

Key variables in `deployments/compose/.env`:

| Variable | Description | Default |
|----------|-------------|---------|
| `POSTGRES_PASSWORD` | Database password | `pepa-secret-change-in-production` |
| `JWT_SECRET` | JWT signing key | `dev-jwt-secret-change-in-production` |
| `ENCRYPTION_KEY` | Secret encryption key (32+ chars) | auto-generated |
| `REDIS_ADDR` | Redis address | `redis:6379` |
| `SERVER_ENV` | Environment (dev/production) | `dev` |

## Configuring AI Provider

AI providers are configured via **Connections** page (not Settings):

1. Go to **Connections** → **Add Connection**
2. Select type: **AI Provider**
3. Choose provider: OpenAI, Anthropic, Groq, LM Studio
4. Enter API key and model name
5. Click **Save**

For local models (LM Studio, Ollama):
- Base URL: `http://host.docker.internal:1234/v1` (LM Studio)
- Base URL: `http://host.docker.internal:11434/v1` (Ollama)
- API Key: any non-empty string (not validated for local models)

## First Steps After Installation

1. **Add a Kubernetes cluster** — Connections → Kubernetes → enter kubeconfig or cluster URL
2. **Register a service** — Services → Add Service → fill in name, owner, repo URL
3. **Install plugins** — Marketplace → install GitLab, GitHub, Jira, or other plugins
4. **Configure CI/CD** — Connections → add GitLab/GitHub token
5. **Set up AI** — Connections → add AI provider → try the AI assistant

## CLI Usage

```bash
# Build the CLI
make build

# Check API health
./bin/pepa health

# Query AI assistant
./bin/pepa ai "What services are running?"

# AI with RAG (knowledge base)
./bin/pepa ai --rag "Explain the deployment pipeline"

# AI with specialist routing
./bin/pepa ai --specialist sre "Why is payment-api failing?"

# Interactive mode
./bin/pepa ai
```

## Common Ports

| Service | Port |
|---------|------|
| Frontend (Next.js) | 3000 |
| API Server | 8088 |
| PostgreSQL | 5432 |
| Redis | 6379 |

## Troubleshooting

**API not reachable**: Check `docker compose ps` — all services should be `running`.

**AI not working**: Verify AI provider is configured in Connections. Check API logs: `docker compose logs pepa-api | grep ai`.

**Plugin not loading**: Check Marketplace page. Plugins require installation from Marketplace before activation.

**Database connection failed**: Verify `POSTGRES_PASSWORD` matches in `.env` and `docker-compose.yml`.
