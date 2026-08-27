// Package main demonstrates a minimal PEPA plugin built with the Go SDK.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// ExamplePlugin is a sample plugin that sends notifications.
type ExamplePlugin struct{}

func (p *ExamplePlugin) Name() string        { return "example" }
func (p *ExamplePlugin) Version() string     { return "0.1.0" }
func (p *ExamplePlugin) Description() string { return "Example notification plugin" }
func (p *ExamplePlugin) PluginType() string  { return "notification" }

func (p *ExamplePlugin) Actions() []string {
	return []string{"send"}
}

func (p *ExamplePlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	switch action {
	case "send":
		return p.send(params, config)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *ExamplePlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{Status: "healthy", Message: "ok"}, nil
}

func (p *ExamplePlugin) send(params []byte, config map[string]string) ([]byte, error) {
	var input struct {
		Channel string `json:"channel"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}

	log.Printf("[example] sending to %s: %s", input.Channel, input.Message)

	return sdk.ActionOutput(map[string]string{
		"channel": input.Channel,
		"status":  "sent",
	})
}

func main() {
	p := &ExamplePlugin{}
	fmt.Printf("Starting %s plugin v%s\n", p.Name(), p.Version())
	sdk.Serve(p)
}
