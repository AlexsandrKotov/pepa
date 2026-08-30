package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func newAICmd() *cobraCmd {
	cmd := &cobraCmd{
		Use:   "ai [question]",
		Short: "Ask the AI assistant from the terminal",
		Long: `Query the PEPA AI assistant directly from the command line.

Examples:
  pepa ai "What services are running?"
  pepa ai --stream "Explain the deployment pipeline"
  pepa ai --specialist sre "Why is payment-api failing?"
  pepa ai --rag "What does the auth service do?"

Environment:
  PEPA_API_URL   API server URL (default: http://localhost:8088)
  PEPA_API_TOKEN API auth token (or uses PEPA_API_USER)`,
		RunE: func(c *cobraCmd, args []string) error {
			if len(args) == 0 {
				// Interactive mode
				return runAIInteractive()
			}

			stream := false
			specialist := ""
			useRAG := false

			// Parse flags from args — stop at first non-flag argument
			// so that queries containing "--" are not misinterpreted.
			var cleanArgs []string
			for i := 0; i < len(args); i++ {
				if len(cleanArgs) > 0 || (!isFlag(args[i])) {
					// Once we see a positional arg, everything else is part of the query
					cleanArgs = append(cleanArgs, args[i:]...)
					break
				}
				switch args[i] {
				case "--stream", "-s":
					stream = true
				case "--rag", "-r":
					useRAG = true
				case "--specialist":
					if i+1 < len(args) {
						specialist = args[i+1]
						i++
					}
				default:
					cleanArgs = append(cleanArgs, args[i])
				}
			}
			query := strings.Join(cleanArgs, " ")

			if query == "" {
				return fmt.Errorf("usage: pepa ai [question] or pepa ai for interactive mode")
			}

			if useRAG {
				return runAIWithRAG(query, stream)
			}
			if specialist != "" {
				return runAIWithSpecialist(query, specialist)
			}
			if stream {
				return runAIStream(query)
			}
			return runAIQuery(query)
		},
	}

	return cmd
}

// isFlag returns true if the argument looks like a known flag.
func isFlag(s string) bool {
	switch s {
	case "--stream", "-s", "--rag", "-r", "--specialist":
		return true
	}
	return false
}

// runAIQuery sends a single query and prints the response.
func runAIQuery(query string) error {
	body := map[string]interface{}{
		"message":      query,
		"enable_tools": true,
	}

	data, err := doRequest("POST", "/api/v1/ai/chat", body)
	if err != nil {
		return fmt.Errorf("AI request failed: %w", err)
	}

	var result struct {
		Response  string `json:"response"`
		ToolCalls int    `json:"tool_calls"`
		Provider  string `json:"provider"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	fmt.Println(result.Response)
	if result.ToolCalls > 0 {
		fmt.Fprintf(os.Stderr, "\n(tools used: %d, provider: %s)\n", result.ToolCalls, result.Provider)
	}
	return nil
}

// runAIStream sends a query and streams the response.
func runAIStream(query string) error {
	body := map[string]interface{}{
		"message":      query,
		"enable_tools": true,
	}

	jsonBody, _ := json.Marshal(body)
	token, err := getToken()
	if err != nil {
		return fmt.Errorf("auth failed: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL+"/api/v1/ai/chat/stream", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("stream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stream failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Read SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Type    string `json:"type"`
			Content string `json:"content"`
			Error   string `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		switch chunk.Type {
		case "text":
			fmt.Print(chunk.Content)
		case "tool_call":
			fmt.Fprintf(os.Stderr, "[tool: %s]", chunk.Content)
		case "error":
			fmt.Fprintf(os.Stderr, "\nError: %s\n", chunk.Error)
		case "done":
			fmt.Println()
		}
	}

	return nil
}

// runAIWithRAG sends a query through the RAG pipeline.
func runAIWithRAG(query string, stream bool) error {
	body := map[string]interface{}{
		"message":       query,
		"top_k":         10,
		"enable_tools":  true,
	}

	endpoint := "/api/v1/rag/chat"
	if stream {
		endpoint = "/api/v1/rag/chat/stream"
	}

	data, err := doRequest("POST", endpoint, body)
	if err != nil {
		return fmt.Errorf("RAG request failed: %w", err)
	}

	var result struct {
		Response string `json:"response"`
		Sources  []struct {
			Source  string  `json:"source"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	fmt.Println(result.Response)
	if len(result.Sources) > 0 {
		fmt.Fprintf(os.Stderr, "\nSources:\n")
		for _, s := range result.Sources {
			preview := s.Content
			if len(preview) > 60 {
				preview = preview[:60] + "..."
			}
			fmt.Fprintf(os.Stderr, "  - [%s] (score: %.3f) %s\n", s.Source, s.Score, preview)
		}
	}
	return nil
}

// runAIWithSpecialist sends a query to a specific specialist agent.
func runAIWithSpecialist(query, specialist string) error {
	body := map[string]interface{}{
		"query": query,
	}

	data, err := doRequest("POST", "/api/v1/agents/route", body)
	if err != nil {
		return fmt.Errorf("specialist request failed: %w", err)
	}

	var result struct {
		Synthesized string `json:"synthesized_answer"`
		Primary     string `json:"primary_specialist"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	fmt.Println(result.Synthesized)
	fmt.Fprintf(os.Stderr, "\n(routed to: %s specialist)\n", result.Primary)
	return nil
}

// runAIInteractive starts an interactive AI chat session.
func runAIInteractive() error {
	fmt.Println("PEPA AI Interactive Mode (type 'exit' or Ctrl+D to quit)")
	fmt.Println("Tips: --rag for knowledge base, --specialist <type> for domain-specific answers")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "exit" || input == "quit" {
			break
		}

		if err := runAIQuery(input); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		fmt.Println()
	}

	return nil
}

// getToken retrieves the auth token for API requests.
func getToken() (string, error) {
	token := os.Getenv("PEPA_API_TOKEN")
	if token != "" {
		return token, nil
	}

	// Fall back to login
	user := os.Getenv("PEPA_API_USER")
	if user == "" {
		user = "admin@example.com"
	}
	password := os.Getenv("PEPA_API_PASSWORD")
	if password == "" {
		password = "admin"
	}

	body := map[string]string{"email": user, "password": password}
	jsonBody, _ := json.Marshal(body)
	resp, err := http.Post(apiURL+"/api/v1/auth/login", "application/json", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse login response: %w", err)
	}
	return result.Token, nil
}
