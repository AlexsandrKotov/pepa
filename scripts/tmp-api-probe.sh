#!/bin/bash
# Probe PEPA API endpoints that frontend pages depend on.
TOKEN=$(cat /tmp/pepa.token)
BASE="http://localhost:8088/api/v1"
probe() {
  local path="$1"
  local out code body
  out=$(curl -s -m 20 -w '\n%{http_code}' "$BASE$path" -H "Authorization: Bearer $TOKEN")
  code=$(echo "$out" | tail -1)
  body=$(echo "$out" | sed '$d' | head -c 220 | tr '\n' ' ')
  printf '%-4s %-52s %s\n' "$code" "$path" "$body"
}
while IFS= read -r p; do [ -n "$p" ] && probe "$p"; done <<'EOF'
/auth/me
/deployments
/environments
/services
/workspaces
/connections
/clusters
/entities
/workflows
/pipelines
/plugins
/marketplace
/settings/get_started
/settings/general
/notifications/rules
/virtualization/proxmox/clusters
/virtualization/proxmox/datastores
/vault/secrets
/vault/bootstrap-status
/security/overview
/security/findings
/compliance/policies
/secret-rotations
/k8s/overview
/services/catalog
/automation/rules
/workflow-board
/cost
/slos
/statuspage
/reports
/repos
/policies
/scorecards
/entities/scorecard-results
/self-service/blueprints
/blueprint-groups
/infrastructure/iac-workspaces
/artifact-registries
EOF
