# PEPA — AI Assistant Guide

## Overview

PEPA has a built-in AI assistant that understands your platform. It can answer questions about services, deployments, pipelines, and infrastructure using real data — not just general knowledge.

## Features

### 1. Conversational AI Chat
Ask questions in natural language:
- "What services are running?"
- "Show me failed pipeline runs from last week"
- "Which clusters are healthy?"

### 2. RAG-Powered Answers
The AI searches the knowledge base (your services, docs, pipeline history) and provides cited answers:
- "What does payment-api do?" → answers from service catalog + documentation
- "What happened during the last incident?" → answers from pipeline run history

### 3. Specialist Agents
Complex queries are routed to specialist agents:

| Specialist | Domain | Example Queries |
|-----------|--------|----------------|
| SRE | Monitoring, incidents, health | "Why is api-gateway returning 500s?" |
| DevOps | Deployments, CI/CD, K8s | "How do I deploy to staging?" |
| Security | Vulnerabilities, RBAC, Vault | "Are there critical CVEs in my services?" |
| Doc | Documentation, knowledge base | "Generate docs for the auth service" |
| Cost | Resource optimization | "Which resources are idle?" |
| General | Multi-domain queries | "Give me a platform health summary" |

### 4. Proactive AI
- **Deployment Risk Assessment** — evaluates risk before deploying
- **Cost Analysis** — finds optimization opportunities
- **Stale Resource Detection** — identifies unused resources
- **Auto-Documentation** — generates service docs from platform data

### 5. Natural Language Workflow Builder
Describe a workflow in plain English, get a YAML workflow definition:
```
POST /api/v1/ai/workflow/build
{
  "description": "Deploy payment-api to staging, run smoke tests, then promote to production if tests pass",
  "environment": "production"
}
```

## API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/api/v1/ai/chat` | POST | Standard AI chat |
| `/api/v1/ai/chat/stream` | POST | Streaming AI chat (SSE) |
| `/api/v1/rag/chat` | POST | RAG chat with knowledge base |
| `/api/v1/rag/chat/stream` | POST | Streaming RAG chat |
| `/api/v1/ai/agents/route` | POST | Route to specialist agent |
| `/api/v1/ai/agents/coordinate` | POST | Multi-agent coordination |
| `/api/v1/ai/agents/specialists` | GET | List available specialists |
| `/api/v1/ai/workflow/build` | POST | Generate workflow from NL |
| `/api/v1/ai/risk/assess` | POST | Assess deployment risk |
| `/api/v1/ai/cost/analyze` | GET | Cost optimization analysis |
| `/api/v1/ai/cost/stale` | GET | Detect stale resources |
| `/api/v1/ai/webhook/suggest` | POST | IDE code suggestions |

## Supported AI Providers

| Provider | Configuration | Notes |
|----------|--------------|-------|
| OpenAI | API key + model | GPT-4o, GPT-4, etc. |
| Anthropic | API key + model | Claude 3.5 Sonnet, etc. |
| Groq | API key + model | Fast inference, Llama/Mixtral |
| LM Studio | Base URL + model | Local, no API key needed |
| Ollama | Base URL + model | Local, fully offline |

## CLI Usage

```bash
# Simple query
pepa ai "List all services"

# With RAG (knowledge base)
pepa ai --rag "What is the deployment process?"

# With specialist
pepa ai --specialist security "Any critical vulnerabilities?"

# Streaming response
pepa ai --stream "Explain the workflow engine"

# Interactive mode
pepa ai
> What services are degraded?
> Show me recent pipeline failures
> exit
```

## Knowledge Base (RAG)

The AI automatically indexes:
- Service catalog (auto-updated on service changes)
- Entity graph (auto-updated on entity changes)
- Pipeline run history (auto-updated on pipeline completion)
- Manual documentation (ingest via API or UI)

To add custom documentation:
```bash
curl -X POST /api/v1/rag/ingest -d '{
  "source": "documentation",
  "type": "runbook",
  "content": "# Payment API Runbook\n..."
}'
```
