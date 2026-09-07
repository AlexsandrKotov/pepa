package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pepa/pepa/internal/events"
	"github.com/pepa/pepa/internal/plugin/engine"
	"github.com/pepa/pepa/internal/repository"
)

// NotificationDispatcher listens for platform events on the event bus and
// dispatches notifications to the appropriate connections based on routing rules.
type NotificationDispatcher struct {
	ruleRepo    *repository.NotificationRuleRepository
	logRepo     *repository.NotificationLogRepository
	connRepo    *repository.ConnectionRepository
	pluginMgr   *engine.Manager
}

// NewNotificationDispatcher creates a new notification dispatcher.
func NewNotificationDispatcher(
	ruleRepo *repository.NotificationRuleRepository,
	logRepo *repository.NotificationLogRepository,
	connRepo *repository.ConnectionRepository,
	pluginMgr *engine.Manager,
) *NotificationDispatcher {
	return &NotificationDispatcher{
		ruleRepo:  ruleRepo,
		logRepo:   logRepo,
		connRepo:  connRepo,
		pluginMgr: pluginMgr,
	}
}

// RegisterHandlers registers the dispatcher as a wildcard listener on the event bus.
func (d *NotificationDispatcher) RegisterHandlers(bus *events.Bus) {
	bus.On("*", d.handleEvent)
	slog.Info("notification dispatcher registered as wildcard event listener")
}

// handleEvent is called for every event on the bus. It finds matching rules
// and dispatches notifications asynchronously.
func (d *NotificationDispatcher) handleEvent(event events.Event) {
	if event.TenantID == "" {
		return
	}

	tenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		slog.Debug("notification dispatcher: invalid tenant ID", "tenant_id", event.TenantID, "error", err)
		return
	}

	// Find matching rules for this event type
	rules, err := d.ruleRepo.FindEnabledByEventType(context.Background(), tenantID, event.Type)
	if err != nil {
		slog.Error("notification dispatcher: failed to find rules", "error", err, "event_type", event.Type)
		return
	}
	if len(rules) == 0 {
		return // no rules match this event — nothing to do
	}

	slog.Debug("notification dispatcher: found matching rules", "event_type", event.Type, "rule_count", len(rules))

	// Process each rule asynchronously to avoid blocking the event bus
	for _, rule := range rules {
		go d.processRule(rule, event)
	}
}

// processRule handles a single rule-event combination: loads the connection,
// renders the template, formats for the provider, executes the plugin, and logs the result.
func (d *NotificationDispatcher) processRule(rule repository.NotificationRule, event events.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tenantID, _ := uuid.Parse(event.TenantID)

	// Load the connection with decrypted credentials
	conn, err := d.connRepo.GetDecrypted(ctx, rule.ConnectionID, tenantID)
	if err != nil {
		slog.Error("notification dispatcher: failed to load connection", "error", err,
			"connection_id", rule.ConnectionID, "rule_id", rule.ID)
		d.logDelivery(tenantID, &rule, event, "", "", "failed", "", fmt.Sprintf("failed to load connection: %v", err))
		return
	}

	// Determine the provider from the connection config
	provider, _ := conn.Config["provider"].(string)
	if provider == "" {
		provider = conn.Name // fallback
	}

	// Build template variables from the event payload
	vars := BuildTemplateVars(event.Type, event.Payload)

	// Render templates
	renderedBody := RenderTemplate(rule.BodyTemplate, vars)
	renderedSubject := ""
	if rule.SubjectTemplate != "" {
		renderedSubject = RenderTemplate(rule.SubjectTemplate, vars)
	}

	// Format for the specific provider
	action, params, err := FormatForProvider(provider, renderedBody, renderedSubject, vars, rule.FormatConfig, conn.Config)
	if err != nil {
		slog.Error("notification dispatcher: formatting failed", "error", err, "provider", provider)
		d.logDelivery(tenantID, &rule, event, renderedSubject, renderedBody, "failed", "", fmt.Sprintf("formatting failed: %v", err))
		return
	}

	// Build plugin config from connection config (string values only)
	pluginConfig := make(map[string]string)
	for k, v := range conn.Config {
		if s, ok := v.(string); ok {
			pluginConfig[k] = s
		}
	}

	// Execute the plugin
	resp, err := d.pluginMgr.Execute(ctx, provider, action, params, pluginConfig)
	if err != nil {
		slog.Error("notification dispatcher: plugin execution failed", "error", err,
			"provider", provider, "action", action, "rule_id", rule.ID)
		d.logDelivery(tenantID, &rule, event, renderedSubject, renderedBody, "failed", "", fmt.Sprintf("plugin execution failed: %v", err))
		return
	}

	if !resp.Success {
		slog.Error("notification dispatcher: plugin returned failure", "error", resp.Error,
			"provider", provider, "action", action, "rule_id", rule.ID)
		d.logDelivery(tenantID, &rule, event, renderedSubject, renderedBody, "failed", "", resp.Error)
		return
	}

	// Success — log delivery
	responseText := string(resp.Output)
	d.logDelivery(tenantID, &rule, event, renderedSubject, renderedBody, "delivered", responseText, "")
	slog.Debug("notification dispatched successfully", "provider", provider, "event_type", event.Type, "rule_id", rule.ID)
}

// logDelivery writes a notification log entry to the database.
func (d *NotificationDispatcher) logDelivery(
	tenantID uuid.UUID,
	rule *repository.NotificationRule,
	event events.Event,
	renderedSubject, renderedBody, status, responseText, errorText string,
) {
	log := &repository.NotificationLog{
		TenantID:        tenantID,
		ConnectionID:    rule.ConnectionID,
		EventType:       event.Type,
		EventPayload:    event.Payload,
		RenderedSubject: renderedSubject,
		RenderedBody:    renderedBody,
		Status:          status,
		ResponseText:    responseText,
		ErrorText:       errorText,
		SentAt:          time.Now().UTC(),
	}
	if rule.ID != uuid.Nil {
		log.RuleID = &rule.ID
	}
	if status == "delivered" {
		now := time.Now().UTC()
		log.DeliveredAt = &now
	}

	// Determine provider from connection config — best effort
	conn, err := d.connRepo.Get(context.Background(), rule.ConnectionID, tenantID)
	if err == nil {
		if p, ok := conn.Config["provider"].(string); ok {
			log.Provider = p
		}
	}

	if err := d.logRepo.Create(context.Background(), log); err != nil {
		slog.Error("notification dispatcher: failed to write delivery log", "error", err)
	}
}

// SendTest sends a test notification through a specific rule's connection.
// Used by the API endpoint POST /notifications/rules/:id/test.
func (d *NotificationDispatcher) SendTest(ctx context.Context, rule *repository.NotificationRule) (string, error) {
	tenantID := rule.TenantID

	// Load the connection with decrypted credentials
	conn, err := d.connRepo.GetDecrypted(ctx, rule.ConnectionID, tenantID)
	if err != nil {
		return "", fmt.Errorf("failed to load connection: %w", err)
	}

	provider, _ := conn.Config["provider"].(string)
	if provider == "" {
		provider = conn.Name
	}

	// Use test payload
	testPayload := map[string]interface{}{
		"event_type":   "test.notification",
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"service_name": "test-service",
		"environment":  "testing",
		"status":       "success",
		"user":         "pepa-admin",
		"message":      "This is a test notification from PEPA",
	}

	vars := BuildTemplateVars("test.notification", testPayload)
	renderedBody := RenderTemplate(rule.BodyTemplate, vars)
	renderedSubject := ""
	if rule.SubjectTemplate != "" {
		renderedSubject = RenderTemplate(rule.SubjectTemplate, vars)
	}

	action, params, err := FormatForProvider(provider, renderedBody, renderedSubject, vars, rule.FormatConfig, conn.Config)
	if err != nil {
		return "", fmt.Errorf("formatting failed: %w", err)
	}

	pluginConfig := make(map[string]string)
	for k, v := range conn.Config {
		if s, ok := v.(string); ok {
			pluginConfig[k] = s
		}
	}

	resp, err := d.pluginMgr.Execute(ctx, provider, action, params, pluginConfig)
	if err != nil {
		return "", fmt.Errorf("plugin execution failed: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("plugin returned failure: %s", resp.Error)
	}

	return string(resp.Output), nil
}

// PreviewTemplate renders a template with sample data and returns the result.
func PreviewTemplate(bodyTemplate string, eventType string) string {
	samplePayload := map[string]interface{}{
		"event_type":     eventType,
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"service_name":   "my-service",
		"environment":    "production",
		"stage":          "prod",
		"status":         "success",
		"user":           "admin",
		"duration":       "2m30s",
		"image_tag":      "v1.2.3",
		"pipeline_name":  "Build & Deploy",
		"source_name":    "github/my-repo",
		"branch":         "main",
		"commit_sha":     "abc123def",
		"workflow_name":  "Deploy Pipeline",
		"steps_completed": 4,
		"steps_total":    4,
		"error":          "",
		"url":            "/deployments/example-uuid",
	}
	return RenderTemplate(bodyTemplate, BuildTemplateVars(eventType, samplePayload))
}

// MarshalPayload is a helper to serialize event payload to JSON for logging.
func MarshalPayload(payload map[string]interface{}) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
}
