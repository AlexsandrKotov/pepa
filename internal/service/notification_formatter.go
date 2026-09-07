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
	params := map[string]interface{}{
		"text":        body,
		"parse_mode":  parseMode,
	}
	// Optional chat_id override
	if cid, ok := formatConfig["chat_id"]; ok {
		params["chat_id"] = cid
	}
	data, err := json.Marshal(params)
	return "send_message", data, err
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
