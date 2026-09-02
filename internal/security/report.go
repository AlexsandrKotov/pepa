package security

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/pepa/pepa/internal/repository"
)

// ReportData holds all data needed to generate a scan report.
type ReportData struct {
	Target    *repository.ScanTarget
	Run       *repository.ScanRun
	Generated time.Time
}

// GenerateJSONReport generates a JSON report from a scan run.
func GenerateJSONReport(target *repository.ScanTarget, run *repository.ScanRun) ([]byte, error) {
	report := map[string]any{
		"report": map[string]any{
			"title":     fmt.Sprintf("Security Scan Report: %s", target.Name),
			"generated": time.Now().UTC().Format(time.RFC3339),
			"version":   "1.0",
		},
		"target": map[string]any{
			"id":            target.ID,
			"name":          target.Name,
			"scanner_type":  target.ScannerType,
			"target_type":   target.TargetType,
			"target_ref":    target.TargetRef,
			"scan_config":   target.ScanConfig,
		},
		"scan": map[string]any{
			"id":           run.ID,
			"status":       run.Status,
			"trigger_type": run.TriggerType,
			"started_at":   run.StartedAt,
			"completed_at": run.CompletedAt,
			"duration_ms":  run.DurationMs,
			"summary":      run.ResultSummary,
			"results":      run.ResultFull,
			"error":        run.ErrorMessage,
		},
	}

	return json.MarshalIndent(report, "", "  ")
}

// GenerateHTMLReport generates an HTML report from a scan run.
func GenerateHTMLReport(w io.Writer, target *repository.ScanTarget, run *repository.ScanRun) error {
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"json": func(v any) string {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		},
		"severityClass": func(sev string) string {
			switch strings.ToLower(sev) {
			case "critical":
				return "severity-critical"
			case "high":
				return "severity-high"
			case "medium":
				return "severity-medium"
			case "low":
				return "severity-low"
			default:
				return "severity-unknown"
			}
		},
		"formatTime": func(t *time.Time) string {
			if t == nil {
				return "N/A"
			}
			return t.Format("2006-01-02 15:04:05 MST")
		},
		"formatDuration": func(ms *int) string {
			if ms == nil {
				return "N/A"
			}
			d := time.Duration(*ms) * time.Millisecond
			if d < time.Second {
				return fmt.Sprintf("%dms", *ms)
			}
			return d.Round(time.Second).String()
		},
	}).Parse(reportTemplate)
	if err != nil {
		return fmt.Errorf("parse report template: %w", err)
	}

	data := ReportData{
		Target:    target,
		Run:       run,
		Generated: time.Now(),
	}

	return tmpl.Execute(w, data)
}

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Security Scan Report - {{.Target.Name}}</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #1a1a2e, #16213e); color: white; padding: 30px; border-radius: 8px; margin-bottom: 30px; }
        .header h1 { font-size: 24px; margin-bottom: 10px; }
        .header .meta { opacity: 0.8; font-size: 14px; }
        .card { background: white; border: 1px solid #e0e0e0; border-radius: 8px; padding: 20px; margin-bottom: 20px; }
        .card h2 { font-size: 18px; margin-bottom: 15px; color: #1a1a2e; border-bottom: 2px solid #eee; padding-bottom: 10px; }
        .status { display: inline-block; padding: 4px 12px; border-radius: 4px; font-size: 12px; font-weight: 600; text-transform: uppercase; }
        .status-completed { background: #d4edda; color: #155724; }
        .status-failed { background: #f8d7da; color: #721c24; }
        .status-running { background: #fff3cd; color: #856404; }
        .status-pending { background: #e2e3e5; color: #383d41; }
        .summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 15px; margin-bottom: 20px; }
        .summary-item { text-align: center; padding: 15px; background: #f8f9fa; border-radius: 8px; }
        .summary-item .value { font-size: 32px; font-weight: 700; }
        .summary-item .label { font-size: 12px; color: #666; text-transform: uppercase; }
        .severity-critical { color: #dc3545; }
        .severity-high { color: #fd7e14; }
        .severity-medium { color: #ffc107; }
        .severity-low { color: #28a745; }
        .severity-unknown { color: #6c757d; }
        table { width: 100%; border-collapse: collapse; margin-top: 15px; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #eee; }
        th { background: #f8f9fa; font-weight: 600; font-size: 13px; text-transform: uppercase; color: #666; }
        pre { background: #f8f9fa; padding: 15px; border-radius: 4px; overflow-x: auto; font-size: 13px; }
        .footer { text-align: center; padding: 20px; color: #666; font-size: 12px; }
        @media print { body { max-width: none; } .card { break-inside: avoid; } }
    </style>
</head>
<body>
    <div class="header">
        <h1>Security Scan Report</h1>
        <div class="meta">
            <div>Target: <strong>{{.Target.Name}}</strong></div>
            <div>Scanner: {{.Target.ScannerType}} | Type: {{.Target.TargetType}} | Ref: {{.Target.TargetRef}}</div>
            <div>Generated: {{.Generated.Format "2006-01-02 15:04:05 MST"}}</div>
        </div>
    </div>

    <div class="card">
        <h2>Scan Information</h2>
        <table>
            <tr><th>Scan ID</th><td>{{.Run.ID}}</td></tr>
            <tr><th>Status</th><td><span class="status status-{{.Run.Status}}">{{.Run.Status}}</span></td></tr>
            <tr><th>Trigger</th><td>{{.Run.TriggerType}}</td></tr>
            <tr><th>Started</th><td>{{formatTime .Run.StartedAt}}</td></tr>
            <tr><th>Completed</th><td>{{formatTime .Run.CompletedAt}}</td></tr>
            <tr><th>Duration</th><td>{{formatDuration .Run.DurationMs}}</td></tr>
        </table>
    </div>

    {{if .Run.ResultSummary}}
    <div class="card">
        <h2>Summary</h2>
        <div class="summary-grid">
            {{range $key, $value := .Run.ResultSummary}}
            <div class="summary-item">
                <div class="value {{$key | severityClass}}">{{$value}}</div>
                <div class="label">{{$key}}</div>
            </div>
            {{end}}
        </div>
    </div>
    {{end}}

    {{if .Run.ErrorMessage}}
    <div class="card">
        <h2>Error</h2>
        <p style="color: #dc3545;">{{.Run.ErrorMessage}}</p>
    </div>
    {{end}}

    {{if .Run.ResultFull}}
    <div class="card">
        <h2>Full Results</h2>
        <pre>{{json .Run.ResultFull}}</pre>
    </div>
    {{end}}

    <div class="footer">
        <p>Generated by PEPA Security Scanner</p>
    </div>
</body>
</html>
`
