# AI/RAG Automation Framework

## 1. Design Philosophy

The AI/RAG (Retrieval-Augmented Generation) Framework provides PEPA with **intelligent automation capabilities** — from a conversational assistant that understands platform telemetry, to automated incident analysis, deployment risk assessment, and documentation generation. The framework is **LLM-agnostic**, supporting any provider from cloud APIs (OpenAI, Anthropic) to self-hosted models (Ollama, vLLM, llama.cpp).

### Key Goals

- **Provider-agnostic** — swap LLM backends without code changes
- **Privacy-first** — support fully local models; no data leaves the cluster
- **Context-aware** — RAG pipeline connects LLMs to real platform data
- **Extensible** — custom tools, agents, and workflows
- **Cost-aware** — model routing based on task complexity and cost optimization

---

## 2. LLM Provider Abstraction

### 2.1 Provider Interface

```go
package ai

// LLMProvider — abstract interface for any LLM backend
type LLMProvider interface {
    // Complete sends a prompt and returns a completion
    Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
    
    // Chat performs a multi-turn conversation
    Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*ChatResponse, error)
    
    // Stream performs a streaming completion (SSE)
    Stream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan *StreamChunk, error)
    
    // Embed generates vector embeddings for text
    Embed(ctx context.Context, texts []string, opts *EmbedOptions) (*EmbedResponse, error)
    
    // Capabilities returns what this provider supports
    Capabilities() ProviderCapabilities
    
    // HealthCheck verifies the provider is accessible
    HealthCheck(ctx context.Context) error
}

// ProviderCapabilities describes what a provider can do
type ProviderCapabilities struct {
    MaxContextLength  int
    SupportsStreaming bool
    SupportsVision    bool
    SupportsFunctionCalling bool
    SupportsJSONMode  bool
    CostPer1KInput    float64  // USD
    CostPer1KOutput   float64  // USD
}

// CompletionRequest
type CompletionRequest struct {
    Prompt      string
    MaxTokens   int
    Temperature float64
    TopP        float64
    Stop        []string
    Model       string  // Override default model
}

// Message for chat conversations
type Message struct {
    Role       string    // system, user, assistant, tool
    Content    string
    Name       string    // Optional name
    ToolCalls  []ToolCall
    ToolCallID string
}

// ChatOptions
type ChatOptions struct {
    Model       string
    MaxTokens   int
    Temperature float64
    TopP        float64
    Tools       []ToolDefinition
    ToolChoice  string  // auto, none, required, or specific tool name
    ResponseFormat string // text, json_object
}

// ToolDefinition — for function calling
type ToolDefinition struct {
    Name        string
    Description string
    Parameters  json.RawMessage  // JSON Schema
}
```

### 2.2 Provider Implementations

```go
// ── OpenAI ──────────────────────────────────────────────────
type OpenAIProvider struct {
    client    *openai.Client
    model     string
    embedModel string
}

func NewOpenAIProvider(config *ProviderConfig) (*OpenAIProvider, error) {
    return &OpenAIProvider{
        client:    openai.NewClient(config.APIKey),
        model:     config.GetOrDefault("model", "gpt-4o"),
        embedModel: config.GetOrDefault("embedModel", "text-embedding-3-small"),
    }, nil
}

// ── Anthropic ───────────────────────────────────────────────
type AnthropicProvider struct {
    client *anthropic.Client
    model  string
}

func NewAnthropicProvider(config *ProviderConfig) (*AnthropicProvider, error) {
    return &AnthropicProvider{
        client: anthropic.NewClient(config.APIKey),
        model:  config.GetOrDefault("model", "claude-sonnet-4-20250514"),
    }, nil
}

// ── Ollama (Local/Self-hosted) ─────────────────────────────
type OllamaProvider struct {
    baseURL string
    model   string
    client  *http.Client
}

func NewOllamaProvider(config *ProviderConfig) (*OllamaProvider, error) {
    return &OllamaProvider{
        baseURL: config.GetOrDefault("baseURL", "http://localhost:11434"),
        model:   config.GetOrDefault("model", "llama3.1:70b"),
        client:  &http.Client{},
    }, nil
}

// ── Azure OpenAI ────────────────────────────────────────────
type AzureOpenAIProvider struct {
    endpoint string
    apiKey   string
    model    string
    client   *http.Client
}

// ── AWS Bedrock ─────────────────────────────────────────────
type BedrockProvider struct {
    region string
    client *bedrockruntime.Client
    model  string
}

// ── Google Gemini ───────────────────────────────────────────
type GeminiProvider struct {
    client *genai.Client
    model  string
}
```

### 2.3 Provider Configuration

```yaml
# ai-config.yaml
apiVersion: pepa.io/v1alpha1
kind: AIConfig
metadata:
  name: default
spec:
  # Default provider for general chat
  defaultProvider: openai
  defaultModel: gpt-4o
  
  # Provider configurations
  providers:
    openai:
      type: openai
      apiKey: "ref:vault://ai/openai-key"
      config:
        model: gpt-4o
        embedModel: text-embedding-3-small
        organization: "org-xxxxx"
        maxTokens: 4096
        temperature: 0.7
    
    anthropic:
      type: anthropic
      apiKey: "ref:vault://ai/anthropic-key"
      config:
        model: claude-sonnet-4-20250514
        maxTokens: 4096
    
    # Self-hosted — no data leaves the cluster
    local:
      type: ollama
      config:
        baseURL: "http://ollama.pepa-system:11434"
        model: llama3.1:70b
        embedModel: nomic-embed-text
    
    # For sensitive data — always use local
    sensitive:
      type: ollama
      config:
        baseURL: "http://ollama.pepa-system:11434"
        model: codellama:70b
  
  # Model routing — choose provider based on task
  routing:
    rules:
      # Simple tasks → cheapest model
      - condition: "task.complexity == 'simple'"
        provider: openai
        model: gpt-4o-mini
      
      # Code generation → best code model
      - condition: "task.type == 'code_generation'"
        provider: anthropic
        model: claude-sonnet-4-20250514
      
      # Sensitive data → local only
      - condition: "task.contains_sensitive_data == true"
        provider: local
        model: llama3.1:70b
      
      # Embedding tasks → dedicated embed model
      - condition: "task.type == 'embedding'"
        provider: openai
        model: text-embedding-3-small
  
  # Cost controls
  costControls:
    dailyBudgetUSD: 50.0
    perUserDailyLimitUSD: 5.0
    alertThresholdPercent: 80
    hardLimitEnabled: true
```

---

## 3. RAG Pipeline Architecture

### 3.1 Overview

```
┌────────────────────────────────────────────────────────────────────────┐
│                        RAG PIPELINE                                     │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    INGESTION PIPELINE                             │   │
│  │                                                                   │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │   │
│  │  │ Document │  │ Chunking │  │ Embedding│  │ Vector Store │   │   │
│  │  │ Loader   │─▶│          │─▶│          │─▶│ (PGvector)   │   │   │
│  │  │          │  │          │  │          │  │              │   │   │
│  │  │ Sources: │  │ Methods: │  │ Models:  │  │ Collections: │   │   │
│  │  │ - K8s    │  │ - Fixed  │  │ - OpenAI │  │ - docs       │   │   │
│  │  │ - Logs   │  │ - Semantic│ │ - Local  │  │ - logs       │   │   │
│  │  │ - Docs   │  │ - Code   │  │ - Custom │  │ - entities   │   │   │
│  │  │ - Events │  │ - Config │  │          │  │ - events     │   │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────────┘   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    QUERY PIPELINE                                 │   │
│  │                                                                   │   │
│  │  User Query                                                       │   │
│  │      │                                                            │   │
│  │      ▼                                                            │   │
│  │  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐  │   │
│  │  │ Query    │    │ Vector   │    │ Context  │    │ LLM      │  │   │
│  │  │ Embed    │───▶│ Search   │───▶│ Assembly │───▶│ Generate │  │   │
│  │  │          │    │ (PGvector│    │          │    │          │  │   │
│  │  │          │    │  + BM25) │    │ - Rerank │    │ - Chat   │  │   │
│  │  │          │    │          │    │ - Filter │    │ - Tools  │  │   │
│  │  │          │    │          │    │ - Format │    │ - Stream │  │   │
│  │  └──────────┘    └──────────┘    └──────────┘    └──────────┘  │   │
│  │                                                        │         │   │
│  │                                                        ▼         │   │
│  │                                                  Response +      │   │
│  │                                                  Citations       │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────┘
```

### 3.2 Document Ingestion

```go
package rag

// DocumentLoader — loads documents from various sources
type DocumentLoader interface {
    Load(ctx context.Context, source *DataSource) ([]*Document, error)
}

// DataSource types
type DataSource struct {
    Type       string            // kubernetes, logs, entity, webhook, file, url
    Config     map[string]string // Source-specific config
    Filter     map[string]string // Filter criteria
    Schedule   string            // Cron schedule for periodic ingestion
}

// ── Kubernetes Resource Loader ──────────────────────────────
type K8sResourceLoader struct {
    client kubernetes.Interface
}

func (l *K8sResourceLoader) Load(ctx context.Context, source *DataSource) ([]*Document, error) {
    // Load Kubernetes resources as documents
    resources := []string{"deployments", "services", "configmaps", "ingresses", "pods"}
    namespace := source.Config["namespace"]  // "*" for all
    
    var docs []*Document
    
    for _, resource := range resources {
        items, err := l.client.Dynamic().Resource(resource).Namespace(namespace).List(ctx, metav1.ListOptions{})
        if err != nil {
            continue
        }
        
        for _, item := range items.Items {
            docs = append(docs, &Document{
                ID:       fmt.Sprintf("k8s/%s/%s/%s", resource, item.GetNamespace(), item.GetName()),
                Source:   "kubernetes",
                Type:     resource,
                Content:  marshalYAML(item.Object),
                Metadata: map[string]string{
                    "namespace": item.GetNamespace(),
                    "name":      item.GetName(),
                    "kind":      item.GetKind(),
                    "cluster":   source.Config["cluster"],
                },
            })
        }
    }
    
    return docs, nil
}

// ── Log Ingestion Loader ────────────────────────────────────
type LogLoader struct {
    // Reads from Loki, Elasticsearch, or CloudWatch
}

func (l *LogLoader) Load(ctx context.Context, source *DataSource) ([]*Document, error) {
    // Query recent error/warning logs
    query := source.Config["query"]  // e.g., "level=error OR level=warn"
    timeRange := parseTimeRange(source.Config["timeRange"])  // e.g., "last_24h"
    
    logs, err := l.queryLogs(ctx, query, timeRange)
    if err != nil {
        return nil, err
    }
    
    // Group related logs into documents
    grouped := groupByServiceAndTimeframe(logs)
    
    var docs []*Document
    for _, group := range grouped {
        docs = append(docs, &Document{
            ID:      fmt.Sprintf("logs/%s/%d", group.Service, group.Timestamp.Unix()),
            Source:  "logs",
            Type:    "log_group",
            Content: formatLogGroup(group),
            Metadata: map[string]string{
                "service":   group.Service,
                "namespace": group.Namespace,
                "level":     group.MaxLevel,
                "count":     fmt.Sprintf("%d", len(group.Entries)),
            },
        })
    }
    
    return docs, nil
}

// ── Entity Graph Loader ─────────────────────────────────────
type EntityLoader struct {
    db *pgx.Pool
}

func (l *EntityLoader) Load(ctx context.Context, source *DataSource) ([]*Document, error) {
    // Load entity data with relationships as context
    entityType := source.Config["entityType"]
    
    rows, _ := l.db.Query(ctx, `
        SELECT e.id, e.name, e.type_key, e.metadata, e.description,
               ARRAY_AGG(
                   rt.display_name || ' → ' || te.name || ' (' || te.type_key || ')'
               ) AS relationships
        FROM entities e
        LEFT JOIN entity_relationships er ON er.source_id = e.id
        LEFT JOIN relationship_types rt ON rt.id = er.relationship_type_id
        LEFT JOIN entities te ON te.id = er.target_id
        WHERE e.type_key = $1 AND e.tenant_id = $2
        GROUP BY e.id
    `, entityType, getTenantID(ctx))
    
    var docs []*Document
    for rows.Next() {
        // Convert each entity to a document
        // ...
    }
    
    return docs, nil
}
```

### 3.3 Chunking Strategy

```go
// Chunker — splits documents into chunks for embedding
type Chunker interface {
    Chunk(doc *Document) ([]*Chunk, error)
}

// Chunk represents a piece of a document
type Chunk struct {
    ID        string
    DocumentID string
    Content   string
    Metadata  map[string]string
    Index     int      // Position in document
    Embedding []float32 // Populated after embedding
}

// ── Semantic Chunker ────────────────────────────────────────
type SemanticChunker struct {
    targetSize  int  // Target tokens per chunk
    overlapSize int  // Overlap between chunks
    separator   string
}

func (c *SemanticChunker) Chunk(doc *Document) ([]*Chunk, error) {
    switch doc.Source {
    case "kubernetes":
        return c.chunkK8sResource(doc)
    case "logs":
        return c.chunkLogGroup(doc)
    case "documentation":
        return c.chunkMarkdown(doc)
    case "code":
        return c.chunkCode(doc)
    default:
        return c.chunkGeneric(doc)
    }
}

// Kubernetes resources: chunk by logical sections
func (c *SemanticChunker) chunkK8sResource(doc *Document) ([]*Chunk, error) {
    // Keep metadata + spec together (usually < 1 chunk)
    // Separate status/events if large
    // ...
}

// Code: chunk by function/class boundaries
func (c *SemanticChunker) chunkCode(doc *Document) ([]*Chunk, error) {
    // Parse AST, split at function boundaries
    // Preserve imports/context in each chunk
    // ...
}
```

### 3.4 Vector Storage (PGvector)

```sql
-- Document store for RAG
CREATE TABLE rag_documents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    
    -- Source tracking
    source      VARCHAR(64) NOT NULL,    -- kubernetes, logs, entity, documentation
    source_type VARCHAR(128),            -- deployment, service, pod, etc.
    source_id   VARCHAR(512),            -- External identifier
    source_url  VARCHAR(1024),           -- Link back to source
    
    -- Content
    content     TEXT NOT NULL,
    metadata    JSONB DEFAULT '{}',
    
    -- Ingestion tracking
    ingested_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    expires_at  TIMESTAMPTZ,             -- Auto-expire old data
    
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Chunk store with embeddings
CREATE TABLE rag_chunks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES rag_documents(id) ON DELETE CASCADE,
    tenant_id   UUID NOT NULL,
    
    -- Chunk content
    content     TEXT NOT NULL,
    chunk_index INTEGER NOT NULL,
    
    -- Embedding vector (1536 dimensions for text-embedding-3-small)
    embedding   vector(1536),
    
    -- Metadata (inherited + chunk-specific)
    metadata    JSONB DEFAULT '{}',
    
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE (document_id, chunk_index)
);

-- Performance indexes
CREATE INDEX idx_rag_chunks_embedding ON rag_chunks 
    USING ivfflat(embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_rag_chunks_doc ON rag_chunks(document_id);
CREATE INDEX idx_rag_chunks_tenant ON rag_chunks(tenant_id);
CREATE INDEX idx_rag_chunks_metadata ON rag_chunks USING GIN(metadata);
CREATE INDEX idx_rag_docs_source ON rag_documents(tenant_id, source, source_type);

-- RLS
ALTER TABLE rag_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE rag_chunks ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_rag_docs ON rag_documents
    USING (tenant_id = current_setting('app.current_tenant')::UUID);
CREATE POLICY tenant_rag_chunks ON rag_chunks
    USING (tenant_id = current_setting('app.current_tenant')::UUID);
```

### 3.5 Query Pipeline

```go
package rag

// QueryPipeline — orchestrates the RAG query flow
type QueryPipeline struct {
    embedder    LLMProvider
    vectorStore *pgx.Pool
    reranker    Reranker
    generator   LLMProvider
    toolRegistry *ToolRegistry
}

func (p *QueryPipeline) Query(ctx context.Context, query *RAGQuery) (*RAGResponse, error) {
    // 1. Generate query embedding
    queryEmbed, err := p.embedder.Embed(ctx, []string{query.Text}, &EmbedOptions{
        Model: "text-embedding-3-small",
    })
    if err != nil {
        return nil, fmt.Errorf("embedding failed: %w", err)
    }
    
    // 2. Vector similarity search
    candidates, err := p.vectorSearch(ctx, queryEmbed.Vectors[0], query.TenantID, query.Filters)
    if err != nil {
        return nil, fmt.Errorf("vector search failed: %w", err)
    }
    
    // 3. BM25 keyword search (hybrid retrieval)
    keywordResults, err := p.keywordSearch(ctx, query.Text, query.TenantID)
    if err != nil {
        return nil, fmt.Errorf("keyword search failed: %w", err)
    }
    
    // 4. Merge and rerank (Reciprocal Rank Fusion)
    merged := reciprocalRankFusion(candidates, keywordResults)
    reranked := p.reranker.Rerank(ctx, query.Text, merged[:50])  // Rerank top 50
    
    // 5. Select top-K context
    topK := reranked[:min(query.TopK, len(reranked))]
    
    // 6. Build context
    context := buildContext(topK, query.MaxContextTokens)
    
    // 7. Generate response with tools
    messages := []Message{
        {Role: "system", Content: systemPrompt(query)},
        {Role: "user", Content: query.Text},
    }
    
    // If tool calling is needed
    if query.EnableTools {
        response, err := p.generator.Chat(ctx, messages, &ChatOptions{
            Model:      query.Model,
            Tools:      p.toolRegistry.GetToolDefinitions(),
            ToolChoice: "auto",
        })
        
        // Handle tool calls iteratively
        for response.HasToolCalls() {
            for _, call := range response.ToolCalls {
                result := p.toolRegistry.ExecuteTool(ctx, call)
                messages = append(messages, response.AssistantMessage())
                messages = append(messages, Message{
                    Role:       "tool",
                    Content:    result.Content,
                    ToolCallID: call.ID,
                })
            }
            response, _ = p.generator.Chat(ctx, messages, &ChatOptions{...})
        }
        
        return &RAGResponse{
            Answer:     response.Content,
            Sources:    buildCitations(topK),
            ToolCalls:  response.ToolCalls,
        }, nil
    }
    
    // Simple generation without tools
    response, _ := p.generator.Chat(ctx, messages, &ChatOptions{
        Model: query.Model,
    })
    
    return &RAGResponse{
        Answer:  response.Content,
        Sources: buildCitations(topK),
    }, nil
}

// RAGQuery
type RAGQuery struct {
    Text            string
    TenantID        string
    TopK            int
    MaxContextTokens int
    Model           string
    Filters         map[string]string  // Filter by source, entity type, etc.
    EnableTools     bool
    ConversationID  string             // For multi-turn context
}

// RAGResponse
type RAGResponse struct {
    Answer    string
    Sources   []Citation
    ToolCalls []ToolCallResult
    TokensUsed TokenUsage
}

type Citation struct {
    DocumentID string
    Source     string
    Content    string
    Score      float64
    URL        string
}
```

---

## 4. AI Tools (Function Calling)

### 4.1 Tool Registry

```go
package ai

// Tool — an action the AI can invoke
type Tool interface {
    Definition() ToolDefinition
    Execute(ctx context.Context, params json.RawMessage) (string, error)
}

// ToolRegistry — manages available AI tools
type ToolRegistry struct {
    tools map[string]Tool
}

// ── Built-in Tools ──────────────────────────────────────────

// GetServiceTopology — retrieves the dependency graph for a service
type GetServiceTopologyTool struct {
    graphEngine *EntityGraphEngine
}

func (t *GetServiceTopologyTool) Definition() ToolDefinition {
    return ToolDefinition{
        Name:        "get_service_topology",
        Description: "Get the dependency graph and relationships for a service, including upstream dependencies and downstream consumers",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "service_name": {"type": "string", "description": "Name of the service"},
                "depth": {"type": "integer", "default": 2, "description": "How many hops to traverse"},
                "relationship_types": {
                    "type": "array",
                    "items": {"type": "string"},
                    "description": "Filter by relationship types (e.g., depends_on, deployed_to)"
                }
            },
            "required": ["service_name"]
        }`),
    }
}

func (t *GetServiceTopologyTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
    var input struct {
        ServiceName string   `json:"service_name"`
        Depth       int      `json:"depth"`
        RelTypes    []string `json:"relationship_types"`
    }
    json.Unmarshal(params, &input)
    
    graph, err := t.graphEngine.GetSubgraph(ctx, input.ServiceName, input.Depth, input.RelTypes)
    if err != nil {
        return "", err
    }
    
    return formatGraphAsText(graph), nil
}

// QueryMetrics — fetches metrics from monitoring plugins
type QueryMetricsTool struct {
    pluginManager *PluginManager
}

func (t *QueryMetricsTool) Definition() ToolDefinition {
    return ToolDefinition{
        Name:        "query_metrics",
        Description: "Query monitoring metrics for a service (Prometheus/PromQL compatible). Returns time-series data.",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "PromQL query"},
                "service": {"type": "string", "description": "Service name"},
                "time_range": {"type": "string", "description": "Time range (e.g., '1h', '24h', '7d')"},
                "step": {"type": "string", "description": "Resolution step (e.g., '1m', '5m')"}
            },
            "required": ["query"]
        }`),
    }
}

// GetRecentIncidents — fetches recent incidents/alerts
type GetRecentIncidentsTool struct {
    pluginManager *PluginManager
}

// SearchDocumentation — searches platform documentation
type SearchDocumentationTool struct {
    ragPipeline *QueryPipeline
}

// GetDeploymentHistory — retrieves deployment history
type GetDeploymentHistoryTool struct {
    pluginManager *PluginManager
}

// TriggerWorkflow — executes a workflow
type TriggerWorkflowTool struct {
    workflowEngine *WorkflowEngine
}

func (t *TriggerWorkflowTool) Definition() ToolDefinition {
    return ToolDefinition{
        Name:        "trigger_workflow",
        Description: "Trigger a workflow execution. Use with caution — this performs actual actions.",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "workflow_name": {"type": "string"},
                "parameters": {"type": "object"},
                "dry_run": {"type": "boolean", "default": false}
            },
            "required": ["workflow_name"]
        }`),
    }
}

// GetCostEstimate — estimates cloud costs
type GetCostEstimateTool struct {
    pluginManager *PluginManager
}

// AnalyzeLogs — queries and analyzes logs
type AnalyzeLogsTool struct {
    logStore *LogStore
}
```

### 4.2 Tool Execution Safety

```go
// Tool execution is governed by RBAC
func (r *ToolRegistry) ExecuteWithAuth(ctx context.Context, toolName string, params json.RawMessage) (string, error) {
    tool, ok := r.tools[toolName]
    if !ok {
        return "", fmt.Errorf("tool %s not found", toolName)
    }
    
    // Check if the user has permission to use this tool
    user := GetUserFromContext(ctx)
    
    // Read-only tools are available to all authenticated users
    readOnlyTools := []string{
        "get_service_topology", "query_metrics", "get_recent_incidents",
        "search_documentation", "get_deployment_history", "analyze_logs",
    }
    
    isReadOnly := contains(readOnlyTools, toolName)
    
    if !isReadOnly {
        // Mutating tools require explicit permission
        allowed, err := r.policyEngine.Evaluate(ctx, &PolicyInput{
            User:     user,
            Action:   "execute",
            Resource: "ai_tool",
            Entity:   map[string]string{"tool": toolName},
        })
        if err != nil || !allowed {
            return "", fmt.Errorf("permission denied for tool %s", toolName)
        }
    }
    
    return tool.Execute(ctx, params)
}
```

---

## 5. AI Assistant Interface

### 5.1 Chat API

```go
// Chat handler — streaming SSE endpoint
func (h *AIHandler) Chat(c *gin.Context) {
    var req ChatRequest
    c.BindJSON(&req)
    
    // Set up SSE streaming
    c.Header("Content-Type", "text/event-stream")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")
    
    flusher, _ := c.Writer.(http.Flusher)
    
    // Build RAG query
    query := &RAGQuery{
        Text:            req.Message,
        TenantID:        getTenantID(c),
        TopK:           10,
        MaxContextTokens: 8000,
        EnableTools:     true,
        ConversationID:  req.ConversationID,
    }
    
    // Stream response
    responseChan := h.ragPipeline.StreamQuery(c.Request.Context(), query)
    
    for chunk := range responseChan {
        switch chunk.Type {
        case "text":
            fmt.Fprintf(c.Writer, "data: %s\n\n", jsonEncode(StreamEvent{
                Type: "text",
                Data: chunk.Content,
            }))
        case "tool_call":
            fmt.Fprintf(c.Writer, "data: %s\n\n", jsonEncode(StreamEvent{
                Type: "tool_call",
                Data: chunk.ToolCall,
            }))
        case "tool_result":
            fmt.Fprintf(c.Writer, "data: %s\n\n", jsonEncode(StreamEvent{
                Type: "tool_result",
                Data: chunk.ToolResult,
            }))
        case "citation":
            fmt.Fprintf(c.Writer, "data: %s\n\n", jsonEncode(StreamEvent{
                Type: "citation",
                Data: chunk.Citation,
            }))
        case "done":
            fmt.Fprintf(c.Writer, "data: %s\n\n", jsonEncode(StreamEvent{
                Type: "done",
                Data: chunk.Metadata,
            }))
        }
        flusher.Flush()
    }
}
```

### 5.2 Frontend Chat Component

```
┌────────────────────────────────────────────────────────────────┐
│  AI Assistant                                              [×] │
├────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ 🤖 What can I help you with?                            │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ 👤 What's causing the 500 errors on payment-api?        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ 🤖 Analyzing payment-api...                             │   │
│  │                                                          │   │
│  │ 🔧 Calling: get_service_topology("payment-api")         │   │
│  │ 🔧 Calling: query_metrics("rate(http_errors_total{      │   │
│  │    service='payment-api'}[5m])")                        │   │
│  │ 🔧 Calling: analyze_logs(service="payment-api",         │   │
│  │    level="error", timeRange="1h")                       │   │
│  │                                                          │   │
│  │ Based on the analysis:                                   │   │
│  │                                                          │   │
│  │ **Root Cause:** The payment-api is experiencing 500      │   │
│  │ errors due to a database connection pool exhaustion.     │   │
│  │ The `payment-db` PostgreSQL instance has reached its     │   │
│  │ max_connections limit (100).                             │   │
│  │                                                          │   │
│  │ **Evidence:**                                            │   │
│  │ - 47 active connections from payment-api (limit: 50)    │   │
│  │ - Error rate spiked at 14:32 UTC (52 errors/min)        │   │
│  │ - Logs show: "too many clients already"                 │   │
│  │                                                          │   │
│  │ **Recommendations:**                                    │   │
│  │ 1. Increase max_connections on payment-db               │   │
│  │ 2. Check for connection leaks in payment-api            │   │
│  │ 3. Consider adding PgBouncer as a connection proxy      │   │
│  │                                                          │   │
│  │ Sources: [payment-api logs] [payment-db metrics]        │   │
│  │          [service topology]                              │   │
│  │                                                          │   │
│  │ [🔄 Trigger: Scale DB] [📋 Create Jira Issue]           │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
│  ┌──────────────────────────────────────────────────────┐ [➤] │
│  │ Ask about services, deployments, incidents...        │      │
│  └──────────────────────────────────────────────────────┘      │
└────────────────────────────────────────────────────────────────┘
```

---

## 6. Automated Intelligence Features

### 6.1 Incident Auto-Analysis

```yaml
# Automated incident analysis pipeline
apiVersion: pepa.io/v1alpha1
kind: AIAutomation
metadata:
  name: incident-analyzer
spec:
  trigger:
    type: entity_event
    config:
      entityType: alert
      event: created
      filter:
        "metadata.severity": ["critical", "high"]
  
  actions:
    # 1. Gather context
    - name: gather-context
      type: parallel
      steps:
        - name: get-topology
          tool: get_service_topology
          params:
            service_name: "{{.trigger.entity.metadata.service}}"
            depth: 3
        - name: get-metrics
          tool: query_metrics
          params:
            service: "{{.trigger.entity.metadata.service}}"
            time_range: "2h"
        - name: get-logs
          tool: analyze_logs
          params:
            service: "{{.trigger.entity.metadata.service}}"
            level: error
            time_range: "2h"
        - name: get-recent-deploys
          tool: get_deployment_history
          params:
            service: "{{.trigger.entity.metadata.service}}"
            limit: 5
        - name: get-recent-incidents
          tool: get_recent_incidents
          params:
            service: "{{.trigger.entity.metadata.service}}"
            limit: 10
    
    # 2. AI analysis
    - name: analyze
      type: ai_chat
      params:
        model: gpt-4o
        systemPrompt: |
          You are an expert SRE assistant. Analyze the incident data and provide:
          1. Root cause analysis
          2. Impact assessment (which services/users are affected)
          3. Recommended remediation steps
          4. Similar past incidents and their resolution
        context: "{{.steps.gather-context.outputs}}"
      
    # 3. Create incident report
    - name: create-report
      type: entity_create
      params:
        entityType: incident_report
        metadata:
          alert_id: "{{.trigger.entity.id}}"
          service: "{{.trigger.entity.metadata.service}}"
          analysis: "{{.steps.analyze.output}}"
          severity: "{{.trigger.entity.metadata.severity}}"
      
    # 4. Notify with AI summary
    - name: notify
      plugin: notification:slack
      action: sendMessage
      params:
        channel: "#incidents"
        message: |
          🚨 **Incident Auto-Analysis: {{.trigger.entity.metadata.service}}**
          
          {{.steps.analyze.output.summary}}
          
          📋 Full report: {{.steps.create-report.entity_url}}
      depends_on: [analyze]
```

### 6.2 Deployment Risk Assessment

```yaml
# Pre-deployment AI risk check
apiVersion: pepa.io/v1alpha1
kind: AIAutomation
metadata:
  name: deployment-risk-assessor
spec:
  trigger:
    type: workflow_step
    config:
      stepType: deploy
      phase: pre_execution
  
  actions:
    - name: assess-risk
      type: ai_chat
      params:
        model: gpt-4o
        systemPrompt: |
          Assess the deployment risk on a scale of 1-10 and explain why.
          Consider: time of day, recent incidents, dependency health,
          change size, test coverage, and historical failure rate.
        tools:
          - get_service_topology
          - query_metrics
          - get_deployment_history
          - get_recent_incidents
        context:
          service: "{{.trigger.service}}"
          version: "{{.trigger.version}}"
          deployer: "{{.trigger.user}}"
      
    - name: gate-decision
      type: condition
      condition: "{{.steps.assess-risk.output.risk_score}} <= 7"
      onFalse:
        action: require_approval
        message: |
          ⚠️ AI Risk Assessment: **{{.steps.assess-risk.output.risk_score}}/10**
          
          {{.steps.assess-risk.output.reasoning}}
          
          Manual approval required due to elevated risk.
```

### 6.3 Documentation Generator

```yaml
# Auto-generate service documentation
apiVersion: pepa.io/v1alpha1
kind: AIAutomation
metadata:
  name: doc-generator
spec:
  trigger:
    type: entity_event
    config:
      entityType: service
      event: created
  
  actions:
    - name: generate-docs
      type: ai_chat
      params:
        model: gpt-4o
        systemPrompt: |
          Generate comprehensive service documentation including:
          - Overview and purpose
          - Architecture and dependencies
          - API endpoints
          - Configuration options
          - Runbook for common issues
          - SLIs/SLOs
        tools:
          - get_service_topology
          - query_metrics
          - get_deployment_history
        outputFormat: markdown
    
    - name: store-docs
      type: entity_update
      params:
        entityType: service
        name: "{{.trigger.entity.name}}"
        updates:
          metadata.documentation: "{{.steps.generate-docs.output}}"
          metadata.documentation_generated_at: "{{now()}}"
          metadata.documentation_model: "gpt-4o"
```

---

## 7. Data Privacy & Security

### 7.1 Data Classification

```go
// DataClassification determines how data is handled
type DataClassification string

const (
    ClassificationPublic     DataClassification = "public"
    ClassificationInternal   DataClassification = "internal"
    ClassificationConfidential DataClassification = "confidential"
    ClassificationRestricted DataClassification = "restricted"
)

// PrivacyPolicy controls what data can be sent to external LLMs
type PrivacyPolicy struct {
    // Data that can NEVER leave the cluster
    RestrictedFromExternalLLM []string{
        "secrets", "tokens", "passwords",
        "pii.emails", "pii.phone_numbers",
        "database.connection_strings",
    }
    
    // Data that CAN be sent to external LLMs
    AllowedForExternalLLM []string{
        "service.names", "metrics", "log.messages",
        "deployment.status", "documentation",
    }
    
    // If data is restricted, force local model
    ForceLocalModelForConfidential bool
}

// PII Scrubber — removes sensitive data before sending to external LLMs
type PIIScrubber struct {
    patterns []*regexp.Regexp
}

func (s *PIIScrubber) Scrub(text string) string {
    // Email addresses
    text = s.emailPattern.ReplaceAllString(text, "[EMAIL_REDACTED]")
    
    // IP addresses (internal)
    text = s.internalIPPattern.ReplaceAllString(text, "[IP_REDACTED]")
    
    // API keys/tokens
    text = s.tokenPattern.ReplaceAllString(text, "[TOKEN_REDACTED]")
    
    // Connection strings
    text = s.connStringPattern.ReplaceAllString(text, "[CONN_STRING_REDACTED]")
    
    return text
}
```

### 7.2 Conversation Storage

```sql
-- AI conversation history
CREATE TABLE ai_conversations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    
    -- Conversation state
    title       VARCHAR(256),
    status      VARCHAR(32) DEFAULT 'active',  -- active, archived
    
    -- Metadata
    metadata    JSONB DEFAULT '{}',
    
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Individual messages
CREATE TABLE ai_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    
    -- Message
    role            VARCHAR(32) NOT NULL,  -- user, assistant, system, tool
    content         TEXT NOT NULL,
    
    -- Tool calls (for assistant messages with function calling)
    tool_calls      JSONB,
    tool_call_id    VARCHAR(256),
    
    -- Token usage
    tokens_input    INTEGER,
    tokens_output   INTEGER,
    model_used      VARCHAR(128),
    provider_used   VARCHAR(64),
    cost_usd        DECIMAL(10, 6),
    
    -- RAG citations
    citations       JSONB,
    
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_ai_conv_user ON ai_conversations(user_id, created_at DESC);
CREATE INDEX idx_ai_msg_conv ON ai_messages(conversation_id, created_at);
```
