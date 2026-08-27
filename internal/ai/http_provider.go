package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// maxResponseSize is the maximum response body we'll read from an LLM provider (10 MB).
const maxResponseSize = 10 << 20

// httpProvider is a lightweight LLM provider that speaks the OpenAI-compatible
// HTTP API.  It works with OpenAI, most self-hosted models (Ollama, vLLM,
// LM Studio), and any endpoint that follows the same request/response shape.
type httpProvider struct {
	name    string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
	mu      sync.Mutex
}

func (p *httpProvider) httpClient() *http.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		return p.client
	}
	p.client = &http.Client{Timeout: 10 * time.Minute}
	return p.client
}

func (p *httpProvider) defaultModel() string {
	if p.model != "" {
		return p.model
	}
	switch p.name {
	case "openai":
		return "gpt-4o-mini"
	case "anthropic":
		return "claude-3-haiku-20240307"
	case "groq":
		return "openai/gpt-oss-120b"
	case "qoder":
		return "qoder-coder"
	case "lmstudio":
		return "local-model"
	case "ollama":
		return "llama3"
	default:
		return "default"
	}
}

func (p *httpProvider) setHeaders(req *http.Request) {
	if p.apiKey != "" {
		if p.name == "anthropic" {
			req.Header.Set("x-api-key", p.apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
	}
	req.Header.Set("Content-Type", "application/json")
}

// Complete sends a single prompt.
func (p *httpProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	model := p.defaultModel()
	if req.Model != "" {
		model = req.Model
	}
	body := map[string]any{
		"model":       model,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
		"messages": []map[string]string{
			{"role": "user", "content": req.Prompt},
		},
	}
	resp, err := p.doChat(ctx, body)
	if err != nil {
		return nil, err
	}
	return &CompletionResponse{
		Text:         resp.Content,
		TokensUsed:   resp.TokensUsed.TotalTokens,
		FinishReason: resp.FinishReason,
		ModelUsed:    resp.ModelUsed,
		ProviderUsed: p.name,
	}, nil
}

// Chat performs a multi-turn conversation.
func (p *httpProvider) Chat(ctx context.Context, messages []Message, opts *ChatOptions) (*ChatResponse, error) {
	model := p.defaultModel()
	if opts != nil && opts.Model != "" {
		model = opts.Model
	}
	msgs := make([]map[string]any, len(messages))
	for i, m := range messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		msgs[i] = msg
	}
	body := map[string]any{
		"model":    model,
		"messages": msgs,
	}
	// Set a reasonable default for max_tokens if not specified.
	// Local models (LM Studio, Ollama) may have low defaults that truncate
	// complex JSON responses needed for intent classification.
	maxTokens := 4096
	if opts != nil && opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	body["max_tokens"] = maxTokens
	if opts != nil {
		if opts.Temperature > 0 {
			body["temperature"] = opts.Temperature
		}
		// Include tools for function calling (OpenAI format)
		if len(opts.Tools) > 0 {
			// Transform ToolDefinition to OpenAI's expected format
			tools := make([]map[string]any, len(opts.Tools))
			for i, t := range opts.Tools {
				tools[i] = map[string]any{
					"type": "function",
					"function": map[string]any{
						"name":        t.Name,
						"description": t.Description,
						"parameters":  t.Parameters,
					},
				}
			}
			body["tools"] = tools
		}
		if opts.ToolChoice != "" {
			body["tool_choice"] = opts.ToolChoice
		}
	}
	return p.doChat(ctx, body)
}

func (p *httpProvider) doChat(ctx context.Context, body map[string]any) (*ChatResponse, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Debug logging for tool calls
	if tools, ok := body["tools"]; ok {
		toolsJSON, _ := json.Marshal(tools)
		log.Printf("[AI Provider] Sending %d tools (%d bytes)", len(tools.([]map[string]any)), len(toolsJSON))
	}

	url := p.baseURL + "/chat/completions"

	// Retry logic for rate limits (429)
	maxRetries := 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		p.setHeaders(req)

		resp, err := p.httpClient().Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == 429 {
			// Rate limited — retry with backoff
			if attempt < maxRetries {
				wait := time.Duration((attempt+1)*3) * time.Second
				log.Printf("[AI Provider] Rate limited (429). Retrying in %v (attempt %d/%d)", wait, attempt+1, maxRetries)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(wait):
				}
				continue
			}
			return nil, fmt.Errorf("provider rate limit exceeded after %d retries: %s", maxRetries, string(respBody))
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("provider returned %d: %s", resp.StatusCode, string(respBody))
		}

		var result struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Choices []struct {
				Message struct {
					Role      string     `json:"role"`
					Content   string     `json:"content"`
					ToolCalls []ToolCall `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
		if len(result.Choices) == 0 {
			return nil, fmt.Errorf("no choices in response")
		}

		// Debug logging for tool calls in response
		if len(result.Choices[0].Message.ToolCalls) > 0 {
			log.Printf("[AI Provider] Received %d tool calls from LLM", len(result.Choices[0].Message.ToolCalls))
			for _, tc := range result.Choices[0].Message.ToolCalls {
				log.Printf("[AI Provider]   - %s(%s)", tc.Function.Name, tc.Function.Arguments)
			}
		} else {
			log.Printf("[AI Provider] No tool calls in response, finish_reason: %s", result.Choices[0].FinishReason)
		}

		return &ChatResponse{
			Content:      stripThinkTags(result.Choices[0].Message.Content),
			ToolCalls:    result.Choices[0].Message.ToolCalls,
			TokensUsed:   TokenUsage{InputTokens: result.Usage.PromptTokens, OutputTokens: result.Usage.CompletionTokens, TotalTokens: result.Usage.TotalTokens},
			ModelUsed:    result.Model,
			ProviderUsed: p.name,
			FinishReason: result.Choices[0].FinishReason,
		}, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// Stream performs a streaming chat completion via SSE.
func (p *httpProvider) Stream(ctx context.Context, messages []Message, opts *ChatOptions) (<-chan *StreamChunk, error) {
	model := p.defaultModel()
	if opts != nil && opts.Model != "" {
		model = opts.Model
	}
	msgs := make([]map[string]any, len(messages))
	for i, m := range messages {
		msg := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		msgs[i] = msg
	}
	body := map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   true,
	}
	// Set a reasonable default for max_tokens if not specified.
	maxTokens := 4096
	if opts != nil && opts.MaxTokens > 0 {
		maxTokens = opts.MaxTokens
	}
	body["max_tokens"] = maxTokens
	if opts != nil {
		if opts.Temperature > 0 {
			body["temperature"] = opts.Temperature
		}
		if len(opts.Tools) > 0 {
			tools := make([]map[string]any, len(opts.Tools))
			for i, t := range opts.Tools {
				tools[i] = map[string]any{
					"type":     "function",
					"function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters},
				}
			}
			body["tools"] = tools
		}
		if opts.ToolChoice != "" {
			body["tool_choice"] = opts.ToolChoice
		}
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	url := p.baseURL + "/chat/completions"

	// Retry logic for rate limits (429)
	maxRetries := 3
	var resp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var reqErr error
		req, reqErr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
		if reqErr != nil {
			return nil, fmt.Errorf("create stream request: %w", reqErr)
		}
		p.setHeaders(req)

		var doErr error
		resp, doErr = p.httpClient().Do(req)
		if doErr != nil {
			return nil, fmt.Errorf("stream request failed: %w", doErr)
		}

		if resp.StatusCode == 429 && attempt < maxRetries {
			wait := time.Duration((attempt+1)*3) * time.Second
			log.Printf("[AI Provider] Stream rate limited (429). Retrying in %v (attempt %d/%d)", wait, attempt+1, maxRetries)
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		break
	}

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("stream provider returned %d: %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan *StreamChunk, 32)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || line == "data: [DONE]" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			var chunk struct {
				Choices []struct {
					Delta struct {
						Role      string     `json:"role"`
						Content   string     `json:"content"`
						ToolCalls []ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			delta := chunk.Choices[0].Delta
			sc := &StreamChunk{}
			if delta.Content != "" {
				sc.Type = "text"
				sc.Content = delta.Content
			} else if len(delta.ToolCalls) > 0 {
				sc.Type = "tool_call"
				sc.ToolCall = &delta.ToolCalls[0]
			} else if chunk.Choices[0].FinishReason != "" {
				sc.Type = "done"
				sc.Metadata = map[string]interface{}{"finish_reason": chunk.Choices[0].FinishReason}
			} else {
				continue
			}
			select {
			case ch <- sc:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// Embed generates embeddings via the /embeddings endpoint.
func (p *httpProvider) Embed(ctx context.Context, texts []string, opts *EmbedOptions) (*EmbedResponse, error) {
	model := "text-embedding-3-small"
	if opts != nil && opts.Model != "" {
		model = opts.Model
	}
	body := map[string]any{"model": model, "input": texts}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	p.setHeaders(req)

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("embeddings returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	vectors := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		vectors[i] = d.Embedding
	}
	return &EmbedResponse{Vectors: vectors, TokensUsed: result.Usage.TotalTokens, ModelUsed: model}, nil
}

// Capabilities returns what this provider supports.
func (p *httpProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		MaxContextLength:        128000,
		SupportsStreaming:       true,
		SupportsVision:          false,
		SupportsFunctionCalling: true,
		SupportsJSONMode:        true,
	}
}

// thinkTagRe matches <think>...</think> blocks (including nested newlines).
var thinkTagRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// stripThinkTags removes <think>...</think> reasoning blocks from LLM output.
// Some models (DeepSeek, Qwen-thinking, etc.) include their internal
// reasoning in these tags — they should not be shown to the user.
func stripThinkTags(s string) string {
	cleaned := thinkTagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(cleaned)
}

// HealthCheck verifies the provider is reachable.
func (p *httpProvider) HealthCheck(ctx context.Context) error {
	url := p.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	p.setHeaders(req)

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("provider unreachable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("provider returned %d", resp.StatusCode)
	}
	return nil
}
