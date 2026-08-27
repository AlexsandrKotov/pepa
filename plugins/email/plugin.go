package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/smtp"
	"regexp"
	"strings"

	"github.com/pepa/pepa/internal/plugin/sdk-go"
	"github.com/pepa/pepa/internal/provider"
)

// headerFieldRe matches characters that must not appear in RFC 2822 header values.
// Disallows CR, LF, and NUL to prevent header injection.
var headerFieldRe = regexp.MustCompile(`[\r\n\000]`)

// EmailPlugin implements provider.Provider for sending emails via SMTP.
type EmailPlugin struct{}

var _ provider.Provider = (*EmailPlugin)(nil)

func (p *EmailPlugin) Name() string        { return "email" }
func (p *EmailPlugin) Version() string     { return "0.1.0" }
func (p *EmailPlugin) Description() string { return "Email notification plugin via SMTP" }
func (p *EmailPlugin) PluginType() string  { return "notification" }

func (p *EmailPlugin) Actions() []string {
	return []string{"send_email"}
}

func (p *EmailPlugin) Execute(ctx context.Context, action string, params []byte, config map[string]string) ([]byte, error) {
	switch action {
	case "send_email":
		return p.sendEmail(ctx, params, config)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

func (p *EmailPlugin) HealthCheck(ctx context.Context) (*provider.HealthStatus, error) {
	return &provider.HealthStatus{
		Status:  "healthy",
		Message: "Email plugin ready — requires SMTP connection config",
	}, nil
}

// sendEmail sends an email via the configured SMTP server.
func (p *EmailPlugin) sendEmail(_ context.Context, params []byte, config map[string]string) ([]byte, error) {
	var req struct {
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Body    string   `json:"body"`
		HTML    bool     `json:"html"`
		Cc      []string `json:"cc"`
	}
	if err := sdk.JSONUnmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	if req.Subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if req.Body == "" {
		return nil, fmt.Errorf("body is required")
	}

	// Validate fields against header injection
	for _, addr := range req.To {
		if headerFieldRe.MatchString(addr) {
			return nil, fmt.Errorf("invalid character in recipient address")
		}
	}
	for _, addr := range req.Cc {
		if headerFieldRe.MatchString(addr) {
			return nil, fmt.Errorf("invalid character in CC address")
		}
	}
	if headerFieldRe.MatchString(req.Subject) {
		return nil, fmt.Errorf("invalid character in subject")
	}

	// SMTP config from connection
	host := config["smtp_host"]
	port := config["smtp_port"]
	user := config["smtp_user"]
	password := config["smtp_password"]
	from := config["from_address"]

	if host == "" {
		return nil, fmt.Errorf("smtp_host is required")
	}
	if port == "" {
		port = "587"
	}
	if from == "" {
		from = user
	}
	if from == "" {
		return nil, fmt.Errorf("from_address or smtp_user is required")
	}
	if headerFieldRe.MatchString(from) {
		return nil, fmt.Errorf("invalid character in from address")
	}

	// Build message
	allRecipients := append(req.To, req.Cc...)
	msg := buildMessage(from, req.To, req.Cc, req.Subject, req.Body, req.HTML)

	addr := net.JoinHostPort(host, port)

	var auth smtp.Auth
	if user != "" && password != "" {
		auth = smtp.PlainAuth("", user, password, host)
	}

	// Use TLS if available
	tlsConfig := &tls.Config{ServerName: host}

	if err := sendMailWithTLS(addr, tlsConfig, from, allRecipients, []byte(msg), auth); err != nil {
		return nil, fmt.Errorf("send email: %w", err)
	}

	return actionOutput(map[string]string{
		"status":     "sent",
		"recipients": strings.Join(req.To, ", "),
	})
}

// buildMessage constructs an RFC 2822 email message.
func buildMessage(from string, to, cc []string, subject, body string, html bool) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	if len(cc) > 0 {
		b.WriteString("Cc: " + strings.Join(cc, ", ") + "\r\n")
	}
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	if html {
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	} else {
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	}
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.String()
}

// sendMailWithTLS sends email using a TLS connection.
// Returns an error if the TLS handshake fails (no plaintext fallback).
func sendMailWithTLS(addr string, tlsConfig *tls.Config, from string, to []string, msg []byte, auth smtp.Auth) error {
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("TLS connection to %s failed: %w (STARTTLS or implicit TLS required)", addr, err)
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, tlsConfig.ServerName)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp rcpt %s: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}

	return client.Quit()
}

func actionOutput(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
