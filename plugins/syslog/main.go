// PEPA Syslog Plugin — forwards logs, events, and audit trail to syslog server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/syslog"
	"net"
	"strings"
	"time"

	sdk "github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// SyslogPlugin implements provider.Provider for syslog forwarding.
type SyslogPlugin struct {
	writer *syslog.Writer
	config map[string]string
}

var _ provider.Provider = (*SyslogPlugin)(nil)

func (p *SyslogPlugin) Name() string    { return "syslog" }
func (p *SyslogPlugin) Version() string { return "0.1.0" }
func (p *SyslogPlugin) Description() string {
	return "Forward PEPA logs, events, and audit trail to syslog server."
}
func (p *SyslogPlugin) PluginType() string { return "logging" }

func (p *SyslogPlugin) Actions() []string {
	return []string{
		"send_log",
		"send_audit",
		"send_event",
		"test_connection",
	}
}

func (p *SyslogPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	// Initialize syslog writer if not already done
	if p.writer == nil {
		if err := p.initWriter(config); err != nil {
			return nil, fmt.Errorf("init syslog writer: %w", err)
		}
	}

	switch action {
	case "send_log":
		return p.sendLog(ctx, params, config)
	case "send_audit":
		return p.sendAudit(ctx, params, config)
	case "send_event":
		return p.sendEvent(ctx, params, config)
	case "test_connection":
		return p.testConnection(ctx, config)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *SyslogPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	if p.writer == nil {
		return &provider.HealthStatus{
			Status:  "unknown",
			Message: "Syslog writer not initialized",
		}, nil
	}

	// Test connection by sending a debug message
	if err := p.writer.Debug("PEPA syslog health check"); err != nil {
		return &provider.HealthStatus{
			Status:  "unhealthy",
			Message: fmt.Sprintf("syslog connection failed: %v", err),
		}, nil
	}

	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Syslog exporter ready",
	}, nil
}

// initWriter initializes the syslog writer.
func (p *SyslogPlugin) initWriter(config map[string]string) error {
	server := config["server"]
	if server == "" {
		return fmt.Errorf("server is required")
	}

	protocol := config["protocol"]
	if protocol == "" {
		protocol = "udp"
	}

	facility := parseFacility(config["facility"])
	tag := config["tag"]
	if tag == "" {
		tag = "pepa"
	}

	writer, err := syslog.Dial(protocol, server, facility, tag)
	if err != nil {
		return fmt.Errorf("dial syslog server: %w", err)
	}

	p.writer = writer
	p.config = config
	return nil
}

// sendLog sends a log message to syslog.
func (p *SyslogPlugin) sendLog(ctx context.Context, params []byte, config map[string]string) ([]byte, error) {
	var req struct {
		Level     string            `json:"level"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields"`
		TraceID   string            `json:"trace_id"`
		Timestamp string            `json:"timestamp"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}

	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	// Format message
	formatted := p.formatMessage(req.Level, req.Message, req.Fields, req.TraceID, req.Timestamp)

	// Send to syslog based on level
	var err error
	switch strings.ToLower(req.Level) {
	case "debug":
		err = p.writer.Debug(formatted)
	case "info":
		err = p.writer.Info(formatted)
	case "warn", "warning":
		err = p.writer.Warning(formatted)
	case "error":
		err = p.writer.Err(formatted)
	case "critical":
		err = p.writer.Crit(formatted)
	default:
		err = p.writer.Info(formatted)
	}

	if err != nil {
		return nil, fmt.Errorf("send to syslog: %w", err)
	}

	return json.Marshal(map[string]string{"status": "sent"})
}

// sendAudit sends an audit event to syslog.
func (p *SyslogPlugin) sendAudit(ctx context.Context, params []byte, config map[string]string) ([]byte, error) {
	var req struct {
		Action     string            `json:"action"`
		EntityType string            `json:"entity_type"`
		EntityID   string            `json:"entity_id"`
		UserID     string            `json:"user_id"`
		Username   string            `json:"username"`
		IPAddress  string            `json:"ip_address"`
		OldValues  map[string]string `json:"old_values"`
		NewValues  map[string]string `json:"new_values"`
		Timestamp  string            `json:"timestamp"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}

	// Format as structured audit message
	auditData := map[string]interface{}{
		"type":        "audit",
		"action":      req.Action,
		"entity_type": req.EntityType,
		"entity_id":   req.EntityID,
		"user_id":     req.UserID,
		"username":    req.Username,
		"ip_address":  req.IPAddress,
		"timestamp":   req.Timestamp,
	}
	if len(req.OldValues) > 0 {
		auditData["old_values"] = req.OldValues
	}
	if len(req.NewValues) > 0 {
		auditData["new_values"] = req.NewValues
	}

	data, _ := json.Marshal(auditData)
	if err := p.writer.Info(string(data)); err != nil {
		return nil, fmt.Errorf("send audit to syslog: %w", err)
	}

	return json.Marshal(map[string]string{"status": "sent"})
}

// sendEvent sends a system event to syslog.
func (p *SyslogPlugin) sendEvent(ctx context.Context, params []byte, config map[string]string) ([]byte, error) {
	var req struct {
		EventType string            `json:"event_type"`
		Source    string            `json:"source"`
		Message   string            `json:"message"`
		Severity  string            `json:"severity"`
		Details   map[string]string `json:"details"`
		Timestamp string            `json:"timestamp"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}

	eventData := map[string]interface{}{
		"type":      "event",
		"event":     req.EventType,
		"source":    req.Source,
		"message":   req.Message,
		"severity":  req.Severity,
		"details":   req.Details,
		"timestamp": req.Timestamp,
	}

	data, _ := json.Marshal(eventData)

	// Send based on severity
	var err error
	switch strings.ToLower(req.Severity) {
	case "critical", "error":
		err = p.writer.Err(string(data))
	case "warning", "warn":
		err = p.writer.Warning(string(data))
	default:
		err = p.writer.Info(string(data))
	}

	if err != nil {
		return nil, fmt.Errorf("send event to syslog: %w", err)
	}

	return json.Marshal(map[string]string{"status": "sent"})
}

// testConnection tests connectivity to the syslog server.
func (p *SyslogPlugin) testConnection(ctx context.Context, config map[string]string) ([]byte, error) {
	server := config["server"]
	protocol := config["protocol"]
	if protocol == "" {
		protocol = "udp"
	}

	// Test network connectivity
	conn, err := net.DialTimeout(protocol, server, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connection test failed: %w", err)
	}
	defer conn.Close()

	// Try to send a test message
	testMsg := "<14>PEPA syslog connection test" // facility local0, severity info
	if _, err := conn.Write([]byte(testMsg)); err != nil {
		return nil, fmt.Errorf("failed to send test message: %w", err)
	}

	return json.Marshal(map[string]string{
		"status":  "ok",
		"message": "Syslog connection test successful",
		"server":  server,
		"protocol": protocol,
	})
}

// formatMessage formats a log message with fields and trace ID.
func (p *SyslogPlugin) formatMessage(level, message string, fields map[string]string, traceID, timestamp string) string {
	format := p.config["format"]
	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		logEntry := map[string]interface{}{
			"level":   level,
			"message": message,
			"time":    timestamp,
		}
		if traceID != "" {
			logEntry["trace_id"] = traceID
		}
		for k, v := range fields {
			logEntry[k] = v
		}
		data, _ := json.Marshal(logEntry)
		return string(data)

	case "rfc5424":
		// RFC5424 format
		ts := timestamp
		if ts == "" {
			ts = time.Now().UTC().Format(time.RFC3339)
		}
		hostname := "pepa"
		appName := "pepa"
		return fmt.Sprintf("<14>1 %s %s %s - - - %s [%s]", ts, hostname, appName, message, level)

	default: // text
		parts := []string{}
		if timestamp != "" {
			parts = append(parts, timestamp)
		}
		parts = append(parts, strings.ToUpper(level))
		if traceID != "" {
			parts = append(parts, fmt.Sprintf("[trace:%s]", traceID))
		}
		parts = append(parts, message)
		for k, v := range fields {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		return strings.Join(parts, " ")
	}
}

// parseFacility converts facility string to syslog.Priority.
func parseFacility(facility string) syslog.Priority {
	switch strings.ToLower(facility) {
	case "local0":
		return syslog.LOG_LOCAL0
	case "local1":
		return syslog.LOG_LOCAL1
	case "local2":
		return syslog.LOG_LOCAL2
	case "local3":
		return syslog.LOG_LOCAL3
	case "local4":
		return syslog.LOG_LOCAL4
	case "local5":
		return syslog.LOG_LOCAL5
	case "local6":
		return syslog.LOG_LOCAL6
	case "local7":
		return syslog.LOG_LOCAL7
	case "user":
		return syslog.LOG_USER
	case "daemon":
		return syslog.LOG_DAEMON
	default:
		return syslog.LOG_LOCAL0
	}
}

func main() {
	sdk.Serve(&SyslogPlugin{})
}
