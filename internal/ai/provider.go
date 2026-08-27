package ai

import (
	"context"
	"encoding/json"
	"errors"
)

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
	MaxContextLength        int
	SupportsStreaming       bool
	SupportsVision          bool
	SupportsFunctionCalling bool
	SupportsJSONMode        bool
	CostPer1KInput          float64 // USD
	CostPer1KOutput         float64 // USD
}

// CompletionRequest
type CompletionRequest struct {
	Prompt      string
	MaxTokens   int
	Temperature float64
	TopP        float64
	Stop        []string
	Model       string // Override default model
}

// CompletionResponse
type CompletionResponse struct {
	Text         string
	TokensUsed   int
	FinishReason string
	ModelUsed    string
	ProviderUsed string
}

// Message for chat conversations
type Message struct {
	Role       string     `json:"role"` // system, user, assistant, tool
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"` // Optional name
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a function call from the LLM
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

// ChatOptions
type ChatOptions struct {
	Model          string           `json:"model"`
	MaxTokens      int              `json:"max_tokens"`
	Temperature    float64          `json:"temperature"`
	TopP           float64          `json:"top_p"`
	Tools          []ToolDefinition `json:"tools,omitempty"`
	ToolChoice     string           `json:"tool_choice,omitempty"`     // auto, none, required, or specific tool name
	ResponseFormat string           `json:"response_format,omitempty"` // text, json_object
}

// ChatResponse
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	TokensUsed   TokenUsage `json:"tokens_used"`
	ModelUsed    string     `json:"model_used"`
	ProviderUsed string     `json:"provider_used"`
	FinishReason string     `json:"finish_reason"`
}

// TokenUsage
type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// StreamChunk
type StreamChunk struct {
	Type       string                 `json:"type"` // text, tool_call, tool_result, citation, done
	Content    string                 `json:"content,omitempty"`
	ToolCall   *ToolCall              `json:"tool_call,omitempty"`
	ToolResult *ToolResult            `json:"tool_result,omitempty"`
	Citation   *Citation              `json:"citation,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ToolResult
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	Error      string `json:"error,omitempty"`
}

// EmbedOptions
type EmbedOptions struct {
	Model string `json:"model"`
}

// EmbedResponse
type EmbedResponse struct {
	Vectors    [][]float32 `json:"vectors"`
	ModelUsed  string      `json:"model_used"`
	TokensUsed int         `json:"tokens_used"`
}

// ToolDefinition — for function calling
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema
}

// Citation
type Citation struct {
	DocumentID string  `json:"document_id"`
	Source     string  `json:"source"`
	Content    string  `json:"content"`
	Score      float64 `json:"score"`
	URL        string  `json:"url,omitempty"`
}

// RAGQuery
type RAGQuery struct {
	Text             string            `json:"text"`
	TenantID         string            `json:"tenant_id"`
	TopK             int               `json:"top_k"`
	MaxContextTokens int               `json:"max_context_tokens"`
	Model            string            `json:"model,omitempty"`
	Filters          map[string]string `json:"filters,omitempty"`
	EnableTools      bool              `json:"enable_tools"`
	ConversationID   string            `json:"conversation_id,omitempty"`
}

// RAGResponse
type RAGResponse struct {
	Answer     string       `json:"answer"`
	Sources    []Citation   `json:"sources,omitempty"`
	ToolCalls  []ToolResult `json:"tool_calls,omitempty"`
	TokensUsed TokenUsage   `json:"tokens_used"`
}

// ProviderConfig
type ProviderConfig struct {
	Type   string            `json:"type"`
	APIKey string            `json:"api_key,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

// Common errors
var (
	ErrProviderNotFound    = errors.New("LLM provider not found")
	ErrProviderUnavailable = errors.New("LLM provider unavailable")
	ErrRateLimitExceeded   = errors.New("rate limit exceeded")
	ErrContextTooLong      = errors.New("context too long")
)
