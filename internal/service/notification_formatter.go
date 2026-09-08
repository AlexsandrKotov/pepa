package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// templateVarRe matches {{ variable }} placeholders in notification templates.
var templateVarRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)

// RenderTemplate replaces {{ variable }} placeholders in a template string
// using values from the provided payload map. Unknown variables are left as-is.
func RenderTemplate(template string, payload map[string]interface{}) string {
	return templateVarRe.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name from {{ name }}
		sub := templateVarRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		key := sub[1]
		if val, ok := payload[key]; ok {
			return fmt.Sprintf("%v", val)
		}
		// Also try nested lookup for dotted keys
		if val := lookupNested(payload, key); val != nil {
			return fmt.Sprintf("%v", val)
		}
		return match // leave unknown vars as-is
	})
}

// lookupNested attempts to resolve dotted keys like "vulnerabilities.critical".
func lookupNested(m map[string]interface{}, key string) interface{} {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 1 {
		if v, ok := m[parts[0]]; ok {
			return v
		}
		return nil
	}
	sub, ok := m[parts[0]]
	if !ok {
		return nil
	}
	subMap, ok := sub.(map[string]interface{})
	if !ok {
		return nil
	}
	return lookupNested(subMap, parts[1])
}

// BuildTemplateVars constructs a standard template variable map from an event payload.
// This ensures common variables like timestamp, event_type, status, etc. are always available.
func BuildTemplateVars(eventType string, payload map[string]interface{}) map[string]interface{} {
	vars := make(map[string]interface{})
	// Copy all payload values
	for k, v := range payload {
		vars[k] = v
	}
	// Add standard variables
	vars["event_type"] = eventType
	vars["timestamp"] = time.Now().UTC().Format(time.RFC3339)
	// Ensure status defaults
	if _, ok := vars["status"]; !ok {
		vars["status"] = "unknown"
	}
	return vars
}

// FormatForProvider renders the final provider-specific message parameters.
// It returns the JSON params to pass to the plugin Execute call and the action name.
func FormatForProvider(provider string, renderedBody string, renderedSubject string, payload map[string]interface{}, formatConfig map[string]any, connConfig map[string]any) (action string, params []byte, err error) {
	switch strings.ToLower(provider) {
	case "slack":
		return formatSlack(renderedBody, formatConfig, connConfig)
	case "telegram":
		return formatTelegram(renderedBody, formatConfig, connConfig)
	case "teams":
		return formatTeams(renderedBody, payload, formatConfig, connConfig)
	case "email":
		return formatEmail(renderedBody, renderedSubject, formatConfig, connConfig)
	case "webhook":
		return formatWebhook(renderedBody, payload, formatConfig, connConfig)
	default:
		return "", nil, fmt.Errorf("unsupported notification provider: %s", provider)
	}
}

func formatSlack(body string, formatConfig map[string]any, connConfig map[string]any) (string, []byte, error) {
	params := map[string]interface{}{
		"text": body,
	}
	// Optional channel override from format_config
	if ch, ok := formatConfig["channel"]; ok {
		params["channel"] = ch
	}
	data, err := json.Marshal(params)
	return "send_message", data, err
}

func formatTelegram(body string, formatConfig map[string]any, connConfig map[string]any) (string, []byte, error) {
	parseMode := "HTML"
	if pm, ok := formatConfig["parse_mode"]; ok {
		if s, ok := pm.(string); ok && s != "" {
			parseMode = s
		}
	}
	// Auto-convert plain text to rich HTML for Telegram
	htmlBody := plainTextToTelegramHTML(body)
	params := map[string]interface{}{
		"text":       htmlBody,
		"parse_mode": parseMode,
	}
	// Optional chat_id override
	if cid, ok := formatConfig["chat_id"]; ok {
		params["chat_id"] = cid
	}
	data, err := json.Marshal(params)
	return "send_message", data, err
}

// plainTextToTelegramHTML converts a plain text notification template into
// beautifully formatted HTML for Telegram. It detects headers, emoji, status
// indicators, key:value pairs, separator lines, and URLs, then wraps them
// in styled HTML for a rich reading experience.
func plainTextToTelegramHTML(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return text
	}

	var result strings.Builder
	isFirst := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Separator lines (━━━━, ────, ----)
		if isSeparatorLine(trimmed) {
			continue // skip visual separators, we use Telegram's own structure
		}

		// Header detection: first non-empty line or lines with only emoji+text
		if isFirst {
			result.WriteString("<b>")
			result.WriteString(escapeHTML(trimmed))
			result.WriteString("</b>")
			isFirst = false
		} else if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
			// URL line — clickable link
			result.WriteString("🔗 <a href=\"")
			result.WriteString(escapeHTML(trimmed))
			result.WriteString("\">Open Link</a>")
		} else if isStatusLine(trimmed) {
			// Lines like "🔴 Critical: 5" or "Status: success" — bold the label
			result.WriteString(formatStatusLine(trimmed))
		} else if isKeyValueLine(trimmed) {
			// Key: Value line — bold the key, keep emoji in value
			result.WriteString(formatKeyValueLine(trimmed))
		} else if isSectionHeader(trimmed) {
			// Lines like "── Details ─" or "── Links ─"
			result.WriteString("\n<b>")
			result.WriteString(escapeHTML(trimmed))
			result.WriteString("</b>")
		} else {
			result.WriteString(escapeHTML(trimmed))
		}

		result.WriteString("\n")
	}

	// Add subtle footer
	result.WriteString("\n<i>🤖 PEPA Platform</i>")

	return result.String()
}

// isSeparatorLine checks if a line is a visual separator (━━━, ───, ---, ===).
func isSeparatorLine(s string) bool {
	cleaned := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, "━", ""), "─", ""), "-", "")
	cleaned = strings.ReplaceAll(cleaned, "=", "")
	return len(strings.TrimSpace(cleaned)) == 0 && len(s) > 2
}

// isStatusLine checks if a line starts with a colored circle emoji or status indicator.
func isStatusLine(s string) bool {
	statusPrefixes := []string{"🔴", "🟠", "🟡", "🟢", "✅", "❌", "⚠️", "🚨", "🚫", "⏪", "🚀"}
	for _, prefix := range statusPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	// Also match "Status: ..." pattern
	if strings.HasPrefix(strings.ToLower(s), "status:") {
		return true
	}
	return false
}

// formatStatusLine formats a status line with bold emoji prefix.
func formatStatusLine(s string) string {
	// Find the emoji at the start and bold the whole line
	return "<b>" + escapeHTML(s) + "</b>"
}

// isKeyValueLine checks if a line contains a "Key: Value" pattern (with optional emoji prefix).
func isKeyValueLine(s string) bool {
	// Strip leading emoji first
	stripped := stripLeadingEmoji(s)
	return strings.Contains(stripped, ": ") && !strings.HasPrefix(strings.TrimSpace(stripped), "http")
}

// formatKeyValueLine bolds the key part and keeps the value normal.
func formatKeyValueLine(s string) string {
	// Handle emoji prefix: "📦 Service: my-app" → "📦 <b>Service:</b> my-app"
	emoji, rest := splitLeadingEmoji(s)
	if strings.Contains(rest, ": ") {
		parts := strings.SplitN(rest, ": ", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			result := emoji + "<b>" + escapeHTML(key) + ":</b> " + escapeHTML(val)
			// Colorize status values
			if strings.ToLower(key) == "status" {
				result = emoji + "<b>" + escapeHTML(key) + ":</b> " + colorizeStatusValue(val)
			}
			return result
		}
	}
	return escapeHTML(s)
}

// isSectionHeader detects lines like "── Details ──────" or "━━ SECTION ━━".
func isSectionHeader(s string) bool {
	return (strings.Contains(s, "──") || strings.Contains(s, "━━")) && len(s) > 4
}

// colorizeStatusValue wraps known status words in colored indicators.
func colorizeStatusValue(val string) string {
	lower := strings.ToLower(val)
	switch {
	case lower == "success" || lower == "delivered" || lower == "passed" || lower == "healthy":
		return "✅ " + escapeHTML(val)
	case lower == "failed" || lower == "error" || lower == "critical":
		return "❌ " + escapeHTML(val)
	case lower == "pending" || lower == "running" || lower == "in_progress":
		return "⏳ " + escapeHTML(val)
	case lower == "cancelled" || lower == "canceled":
		return "🚫 " + escapeHTML(val)
	default:
		return escapeHTML(val)
	}
}

// stripLeadingEmoji removes the first emoji character(s) from a string.
func stripLeadingEmoji(s string) string {
	for i, r := range s {
		if r > 0x1F000 || (r >= 0x2600 && r <= 0x27BF) {
			// This is likely an emoji, continue to find where emoji sequence ends
			continue
		}
		if i > 0 {
			return strings.TrimSpace(s[i:])
		}
		return s
	}
	return s
}

// splitLeadingEmoji separates leading emoji from the rest of the string.
func splitLeadingEmoji(s string) (emoji string, rest string) {
	for i, r := range s {
		if r > 0x1F000 || (r >= 0x2600 && r <= 0x27BF) || r == 0xFE0F {
			continue // emoji or variation selector
		}
		if i > 0 {
			return s[:i], strings.TrimSpace(s[i:])
		}
		return "", s
	}
	return s, ""
}

// escapeHTML escapes HTML special characters for Telegram HTML mode.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func formatTeams(body string, payload map[string]interface{}, formatConfig map[string]any, connConfig map[string]any) (string, []byte, error) {
	// Teams plugin wraps text in a MessageCard automatically
	params := map[string]interface{}{
		"text": body,
	}
	data, err := json.Marshal(params)
	return "send_message", data, err
}

func formatEmail(body string, subject string, formatConfig map[string]any, connConfig map[string]any) (string, []byte, error) {
	// Extract recipients from connection config or format_config
	var to []string
	if recipients, ok := formatConfig["to"]; ok {
		switch v := recipients.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					to = append(to, s)
				}
			}
		case string:
			to = strings.Split(v, ",")
			for i := range to {
				to[i] = strings.TrimSpace(to[i])
			}
		}
	}
	if len(to) == 0 {
		// Fallback to connection config
		if recipients, ok := connConfig["recipients"]; ok {
			switch v := recipients.(type) {
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						to = append(to, s)
					}
				}
			case string:
				to = strings.Split(v, ",")
				for i := range to {
					to[i] = strings.TrimSpace(to[i])
				}
			}
		}
	}
	if len(to) == 0 {
		return "", nil, fmt.Errorf("no recipients configured for email notification")
	}

	if subject == "" {
		subject = "PEPA Notification"
	}

	// Build an HTML body for better readability
	htmlBody := buildHTMLEmail(body)

	params := map[string]interface{}{
		"to":      to,
		"subject": subject,
		"body":    htmlBody,
		"html":    true,
	}
	data, err := json.Marshal(params)
	return "send_email", data, err
}

func formatWebhook(body string, payload map[string]interface{}, formatConfig map[string]any, connConfig map[string]any) (string, []byte, error) {
	webhookPayload := map[string]interface{}{
		"event_type": payload["event_type"],
		"timestamp":  payload["timestamp"],
		"message":    body,
		"data":       payload,
	}

	params := map[string]interface{}{
		"payload": webhookPayload,
		"method":  "POST",
		"headers": map[string]string{
			"X-PEPA-Event": fmt.Sprintf("%v", payload["event_type"]),
		},
	}
	// Optional URL override from format_config
	if url, ok := formatConfig["webhook_url"]; ok {
		params["url"] = url
	}
	data, err := json.Marshal(params)
	return "send_webhook", data, err
}

// buildHTMLEmail wraps plain text notification body in a simple styled HTML template.
func buildHTMLEmail(text string) string {
	// Convert newlines to <br>
	html := strings.ReplaceAll(text, "\n", "<br>")
	return `<!DOCTYPE html>
<html>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; padding: 20px; background: #f5f5f5;">
<div style="max-width: 600px; margin: 0 auto; background: white; border-radius: 8px; padding: 24px; border: 1px solid #e0e0e0;">
<div style="margin-bottom: 16px;">
<span style="font-weight: 600; color: #1a1a1a;">PEPA Platform</span>
</div>
<div style="color: #333; line-height: 1.6; font-size: 14px;">
` + html + `
</div>
<div style="margin-top: 20px; padding-top: 12px; border-top: 1px solid #eee; font-size: 12px; color: #999;">
Sent by PEPA Notification Center
</div>
</div>
</body>
</html>`
}

// DefaultTemplates returns the default body templates for each event type.
// These are used when creating new notification rules.
func DefaultTemplates() map[string]string {
	return map[string]string{
		"deployment.succeeded": "✅ PEPA Deployment Succeeded\nService: {{ service_name }}\nEnvironment: {{ environment }}\nStage: {{ stage }}\nImage: {{ image_tag }}\nDuration: {{ duration }}\nTriggered by: {{ user }}",
		"deployment.failed":    "❌ PEPA Deployment FAILED\nService: {{ service_name }}\nEnvironment: {{ environment }}\nStage: {{ stage }}\nError: {{ error }}\nLogs: {{ url }}",
		"deployment.promoted":  "🚀 PEPA Deployment Promoted\nService: {{ service_name }}\nFrom: {{ from_stage }} → To: {{ to_stage }}\nImage: {{ image_tag }}\nBy: {{ user }}",
		"deployment.rolled_back": "⏪ PEPA Deployment Rolled Back\nService: {{ service_name }}\nEnvironment: {{ environment }}\nBy: {{ user }}",
		"deployment.cancelled": "🚫 PEPA Deployment Cancelled\nService: {{ service_name }}\nEnvironment: {{ environment }}\nBy: {{ user }}",
		"pipeline_run.completed": "🔧 Pipeline '{{ pipeline_name }}' completed\nStatus: {{ status }}\nSource: {{ source_name }}\nBranch: {{ branch }}\nCommit: {{ commit_sha }}\nDuration: {{ duration }}\nSteps: {{ steps_completed }}/{{ steps_total }}",
		"pipeline_run.step_completed": "Step '{{ step_name }}' completed in pipeline '{{ pipeline_name }}'\nStatus: {{ status }}\nDuration: {{ step_duration }}",
		"scan.completed": "🛡️ Security Scan completed\nTarget: {{ target_name }}\nScanner: {{ scanner_type }}\nVulnerabilities: Critical={{ critical }}, High={{ high }}, Medium={{ medium }}, Low={{ low }}\nQuality Gate: {{ quality_gate }}\nDuration: {{ duration }}",
		"scan.failed":    "⚠️ Security Scan FAILED\nTarget: {{ target_name }}\nScanner: {{ scanner_type }}\nError: {{ error }}",
		"finding.critical": "🚨 CRITICAL vulnerability found!\nTitle: {{ finding_title }}\nResource: {{ resource_name }}\nSeverity: {{ severity }}\nIdentifier: {{ identifier }}",
		"workflow.completed": "✅ Workflow '{{ workflow_name }}' completed\nDuration: {{ duration }}\nSteps: {{ steps_completed }}/{{ steps_total }}",
		"workflow.failed": "❌ Workflow '{{ workflow_name }}' FAILED\nError: {{ error }}\nFailed step: {{ failed_step }}",
		"service.created":   "📦 New service registered: {{ service_name }}\nType: {{ service_type }}\nOwner: {{ owner }}",
		"service.updated":   "📝 Service updated: {{ service_name }}\nType: {{ service_type }}",
		"service.deleted":   "🗑️ Service deleted: {{ service_name }}",
		"entity.created":    "📦 Entity created: {{ entity_name }}\nType: {{ entity_type }}",
		"entity.updated":    "📝 Entity updated: {{ entity_name }}\nType: {{ entity_type }}",
		"entity.deleted":    "🗑️ Entity deleted: {{ entity_name }}",
		"connection.created": "🔗 Connection created: {{ connection_name }}\nType: {{ connection_type }}",
		"connection.deleted": "🔗 Connection deleted: {{ connection_name }}",
		"vm.created":        "🖥️ VM created: {{ vm_name }}\nProvider: {{ provider }}\nType: {{ vm_type }}\nIP: {{ ip_address }}",
		"vm.deleted":        "🗑️ VM deleted: {{ vm_name }}\nProvider: {{ provider }}",
		"vm.action":         "🔧 VM action: {{ action }} on {{ vm_name }}\nProvider: {{ provider }}\nStatus: {{ status }}",
		"user.created":      "👤 User created: {{ username }}\nRole: {{ role }}",
		"user.deleted":      "👤 User deleted: {{ username }}",
		"role.assigned":     "🔑 Role '{{ role }}' assigned to {{ username }}",
		"role.revoked":      "🔑 Role '{{ role }}' revoked from {{ username }}",
		"scorecard.created": "📋 Scorecard created: {{ scorecard_name }}",
		"scorecard.updated": "📋 Scorecard updated: {{ scorecard_name }}",
		"blueprint.created": "📐 Blueprint created: {{ blueprint_name }}",
	}
}

// TemplatePreset represents a pre-designed notification template that users
// can select and customize.
type TemplatePreset struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Category    string   `json:"category"`
	EventTypes  []string `json:"event_types"`
	Body        string   `json:"body"`
	Subject     string   `json:"subject,omitempty"`
}

// TemplatePresets returns a collection of cleanly designed notification
// templates that users can select and customize for their needs.
// Emojis are used sparingly — only on headers and critical status lines.
func TemplatePresets() []TemplatePreset {
	return []TemplatePreset{
		// ── Deployment presets ──────────────────────────────────
		{
			ID:          "deploy-success",
			Name:        "Deployment Success",
			Description: "Clean success alert with service details",
			Icon:        "✅",
			Category:    "Deployments",
			EventTypes:  []string{"deployment.succeeded"},
			Subject:     "Deploy: {{ service_name }} → {{ environment }}",
			Body: `✅ Deployment Succeeded

Service: {{ service_name }}
Environment: {{ environment }}
Image: {{ image_tag }}
Duration: {{ duration }}
Triggered by: {{ user }}
{{ timestamp }}`,
		},
		{
			ID:          "deploy-fail",
			Name:        "Deployment Failure",
			Description: "Critical failure alert with error details",
			Icon:        "❌",
			Category:    "Deployments",
			EventTypes:  []string{"deployment.failed"},
			Subject:     "Deploy FAILED: {{ service_name }}",
			Body: `❌ Deployment FAILED

Service: {{ service_name }}
Environment: {{ environment }}
Stage: {{ stage }}
Error: {{ error }}
User: {{ user }}
{{ timestamp }}

Details: {{ url }}`,
		},
		{
			ID:          "deploy-all",
			Name:        "All Deployment Events",
			Description: "Universal template for any deployment event",
			Icon:        "🚀",
			Category:    "Deployments",
			EventTypes:  []string{"deployment.created", "deployment.succeeded", "deployment.failed", "deployment.promoted", "deployment.rolled_back", "deployment.cancelled"},
			Subject:     "Deploy [{{ status }}]: {{ service_name }}",
			Body: `Deployment Event

Event: {{ event_type }}
Service: {{ service_name }}
Environment: {{ environment }}
Status: {{ status }}
User: {{ user }}
Duration: {{ duration }}
{{ timestamp }}

Details: {{ url }}`,
		},
		// ── Pipeline presets ────────────────────────────────────
		{
			ID:          "pipeline-complete",
			Name:        "Pipeline Complete",
			Description: "Pipeline run summary with steps breakdown",
			Icon:        "🔧",
			Category:    "Pipelines",
			EventTypes:  []string{"pipeline_run.completed"},
			Subject:     "Pipeline: {{ pipeline_name }} — {{ status }}",
			Body: `Pipeline Completed

Pipeline: {{ pipeline_name }}
Status: {{ status }}
Branch: {{ branch }}
Source: {{ source_name }}
Commit: {{ commit_sha }}
Duration: {{ duration }}
Steps: {{ steps_completed }}/{{ steps_total }}
{{ timestamp }}`,
		},
		{
			ID:          "pipeline-fail",
			Name:        "Pipeline Failure",
			Description: "Pipeline failure with debugging info",
			Icon:        "🔴",
			Category:    "Pipelines",
			EventTypes:  []string{"pipeline_run.completed"},
			Subject:     "Pipeline FAILED: {{ pipeline_name }}",
			Body: `❌ Pipeline FAILED

Pipeline: {{ pipeline_name }}
Branch: {{ branch }}
Source: {{ source_name }}
Commit: {{ commit_sha }}
Error: {{ error }}
Steps: {{ steps_completed }}/{{ steps_total }}
{{ timestamp }}

Details: {{ url }}`,
		},
		// ── Security presets ────────────────────────────────────
		{
			ID:          "scan-report",
			Name:        "Security Scan Report",
			Description: "Vulnerability summary with quality gate",
			Icon:        "🛡️",
			Category:    "Security",
			EventTypes:  []string{"scan.completed"},
			Subject:     "Scan: {{ target_name }} — {{ quality_gate }}",
			Body: `🛡️ Security Scan Report

Target: {{ target_name }}
Scanner: {{ scanner_type }}
Quality Gate: {{ quality_gate }}

Critical: {{ critical }} | High: {{ high }} | Medium: {{ medium }} | Low: {{ low }}

Duration: {{ duration }}
{{ timestamp }}`,
		},
		{
			ID:          "critical-finding",
			Name:        "Critical Vulnerability",
			Description: "Urgent alert for critical findings",
			Icon:        "🚨",
			Category:    "Security",
			EventTypes:  []string{"finding.critical"},
			Subject:     "CRITICAL: {{ finding_title }}",
			Body: `🚨 CRITICAL VULNERABILITY

Title: {{ finding_title }}
Resource: {{ resource_name }}
Severity: {{ severity }}
Identifier: {{ identifier }}
{{ timestamp }}

Immediate action required!`,
		},
		// ── Workflow presets ────────────────────────────────────
		{
			ID:          "workflow-complete",
			Name:        "Workflow Complete",
			Description: "Workflow execution summary",
			Icon:        "⚡",
			Category:    "Workflows",
			EventTypes:  []string{"workflow.completed"},
			Subject:     "Workflow: {{ workflow_name }} completed",
			Body: `✅ Workflow Completed

Workflow: {{ workflow_name }}
Status: {{ status }}
Duration: {{ duration }}
Steps: {{ steps_completed }}/{{ steps_total }}
{{ timestamp }}`,
		},
		{
			ID:          "workflow-fail",
			Name:        "Workflow Failure",
			Description: "Workflow failure with error details",
			Icon:        "💥",
			Category:    "Workflows",
			EventTypes:  []string{"workflow.failed"},
			Subject:     "Workflow FAILED: {{ workflow_name }}",
			Body: `❌ Workflow FAILED

Workflow: {{ workflow_name }}
Error: {{ error }}
Failed step: {{ failed_step }}
{{ timestamp }}`,
		},
		// ── Service presets ─────────────────────────────────────
		{
			ID:          "service-lifecycle",
			Name:        "Service Lifecycle",
			Description: "Track all service create/update/delete events",
			Icon:        "📦",
			Category:    "Services",
			EventTypes:  []string{"service.created", "service.updated", "service.deleted"},
			Subject:     "Service [{{ event_type }}]: {{ service_name }}",
			Body: `Service Event

Event: {{ event_type }}
Service: {{ service_name }}
Type: {{ service_type }}
Owner: {{ owner }}
{{ timestamp }}`,
		},
		// ── Infrastructure presets ──────────────────────────────
		{
			ID:          "vm-lifecycle",
			Name:        "VM Lifecycle",
			Description: "Virtual machine create/delete/action events",
			Icon:        "🖥️",
			Category:    "Infrastructure",
			EventTypes:  []string{"vm.created", "vm.deleted", "vm.action"},
			Subject:     "VM [{{ event_type }}]: {{ vm_name }}",
			Body: `VM Event

Event: {{ event_type }}
VM: {{ vm_name }}
Provider: {{ provider }}
Status: {{ status }}
IP: {{ ip_address }}
{{ timestamp }}`,
		},
		// ── Compact preset ──────────────────────────────────────
		{
			ID:          "compact-universal",
			Name:        "Compact Universal",
			Description: "Short one-liner for high-volume events",
			Icon:        "📎",
			Category:    "General",
			EventTypes:  []string{"deployment.succeeded", "deployment.failed", "pipeline_run.completed", "scan.completed", "workflow.completed"},
			Subject:     "PEPA: {{ event_type }}",
			Body:        "{{ event_type }} | {{ service_name }}{{ pipeline_name }} | {{ status }} | {{ duration }} | {{ timestamp }}",
		},
		// ── Detailed preset ─────────────────────────────────────
		{
			ID:          "detailed-universal",
			Name:        "Detailed Universal",
			Description: "Full details with all available information",
			Icon:        "📄",
			Category:    "General",
			EventTypes:  []string{"deployment.succeeded", "deployment.failed", "pipeline_run.completed", "scan.completed", "workflow.completed"},
			Subject:     "PEPA: {{ event_type }}",
			Body: `PEPA Notification

Event: {{ event_type }}
Status: {{ status }}
Time: {{ timestamp }}

Service: {{ service_name }}
Environment: {{ environment }}
User: {{ user }}
Duration: {{ duration }}
Image: {{ image_tag }}
Branch: {{ branch }}
Commit: {{ commit_sha }}

{{ url }}`,
		},
	}
}

// EventCategories returns the event types grouped by category for the UI dropdown.
func EventCategories() map[string][]string {
	return map[string][]string{
		"Deployments": {
			"deployment.created", "deployment.started", "deployment.succeeded",
			"deployment.failed", "deployment.promoted", "deployment.rolled_back", "deployment.cancelled",
		},
		"Pipelines": {
			"pipeline_run.created", "pipeline_run.started",
			"pipeline_run.completed", "pipeline_run.step_completed",
		},
		"Security": {
			"scan.started", "scan.completed", "scan.failed",
			"finding.critical", "finding.resolved",
		},
		"Workflows": {
			"workflow.created", "workflow.running", "workflow.completed",
			"workflow.failed", "workflow.step_failed",
		},
		"Services": {
			"service.created", "service.updated", "service.deleted",
			"service.deployment_completed",
		},
		"Entities": {
			"entity.created", "entity.updated", "entity.deleted",
		},
		"Connections": {
			"connection.created", "connection.deleted",
			"connection.test_passed", "connection.test_failed",
		},
		"Plugins": {
			"plugin.enabled", "plugin.disabled", "plugin.installed",
		},
		"Infrastructure": {
			"vm.created", "vm.deleted", "vm.action", "docker.service_deployed",
		},
		"Scorecards": {
			"scorecard.created", "scorecard.updated",
		},
		"Auth & RBAC": {
			"user.created", "user.deleted", "role.assigned", "role.revoked",
		},
	}
}
