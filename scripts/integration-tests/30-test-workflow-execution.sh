#!/usr/bin/env bash
# 30-test-workflow-execution.sh — Workflow CRUD + execution with input parameters
# Covers: POST /workflows, GET /workflows, GET /workflows/:id, PUT /workflows/:id,
#         POST /workflows/:id/execute, GET /workflows/:id/executions, DELETE /workflows/:id

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 30: Workflow CRUD & Execution"

pepa_login 2>/dev/null || true

# ── 30.1 List workflows ────────────────────────────────────────────────────
log_test_start "30.1" "GET /workflows returns workflow list"
pepa_api GET "/workflows"
if [[ "$API_STATUS" == "200" ]]; then
    WF_COUNT=$(echo "$API_RESPONSE" | jq '.workflows // .items // [] | length' 2>/dev/null)
    log_test "30.1" "Workflows: $WF_COUNT found" "pass"
else
    log_test "30.1" "List workflows" "fail" "HTTP $API_STATUS"
fi

# ── 30.2 Create workflow ───────────────────────────────────────────────────
log_test_start "30.2" "POST /workflows creates a new workflow"
WF_BODY='{
    "name": "test-deploy-workflow",
    "source": "visual",
    "spec": {
        "triggers": [{"type": "manual", "config": {}}],
        "settings": {"timeout": "30m", "concurrency": 1, "onConflict": "queue"},
        "steps": [
            {
                "name": "build",
                "description": "Build application",
                "action": "build",
                "parameters": {"image_tag": "{{ input.image_tag }}"}
            },
            {
                "name": "deploy",
                "description": "Deploy to environment",
                "action": "deploy",
                "parameters": {"environment": "{{ input.environment }}"},
                "depends_on": ["build"]
            }
        ]
    }
}'
pepa_api POST "/workflows" "$WF_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    WF_ID=$(echo "$API_RESPONSE" | jq -r '.id // empty')
    WF_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
    log_test "30.2" "Workflow created: id=${WF_ID:0:8}..., name=$WF_NAME" "pass"
else
    log_test "30.2" "Create workflow" "fail" "HTTP $API_STATUS"
    WF_ID=""
fi

# ── 30.3 Get workflow by ID ────────────────────────────────────────────────
if [[ -n "$WF_ID" ]]; then
    log_test_start "30.3" "GET /workflows/:id returns workflow details"
    pepa_api GET "/workflows/$WF_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        GOT_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        GOT_ENABLED=$(echo "$API_RESPONSE" | jq -r '.is_enabled // empty')
        GOT_VERSION=$(echo "$API_RESPONSE" | jq -r '.version // empty')
        log_test "30.3" "Workflow: name=$GOT_NAME, enabled=$GOT_ENABLED, version=$GOT_VERSION" "pass"
    else
        log_test "30.3" "Get workflow" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "30.3" "Get workflow" "skip" "no workflow"
fi

# ── 30.4 Update workflow ───────────────────────────────────────────────────
if [[ -n "$WF_ID" ]]; then
    log_test_start "30.4" "PUT /workflows/:id updates workflow"
    pepa_api PUT "/workflows/$WF_ID" '{"name": "test-deploy-workflow-updated"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        log_test "30.4" "Workflow updated: name=$UPD_NAME" "pass"
    else
        log_test "30.4" "Update workflow" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "30.4" "Update workflow" "skip" "no workflow"
fi

# ── 30.5 Execute workflow with input parameters ────────────────────────────
if [[ -n "$WF_ID" ]]; then
    log_test_start "30.5" "POST /workflows/:id/execute with input params"
    EXEC_BODY='{"trigger_payload": {"image_tag": "v2.0.0", "environment": "staging"}}'
    pepa_api POST "/workflows/$WF_ID/execute" "$EXEC_BODY"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        EXEC_ID=$(echo "$API_RESPONSE" | jq -r '.id // .execution_id // empty')
        EXEC_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
        log_test "30.5" "Execution started: id=${EXEC_ID:0:8}..., status=$EXEC_STATUS" "pass"
    else
        log_test "30.5" "Execute workflow" "fail" "HTTP $API_STATUS"
        EXEC_ID=""
    fi
else
    log_test "30.5" "Execute workflow" "skip" "no workflow"
    EXEC_ID=""
fi

# ── 30.6 List workflow executions ──────────────────────────────────────────
if [[ -n "$WF_ID" ]]; then
    log_test_start "30.6" "GET /workflows/:id/executions lists executions"
    pepa_api GET "/workflows/$WF_ID/executions"
    if [[ "$API_STATUS" == "200" ]]; then
        EXEC_COUNT=$(echo "$API_RESPONSE" | jq '.executions // .items // [] | length' 2>/dev/null)
        log_test "30.6" "Executions: $EXEC_COUNT found" "pass"
    else
        log_test "30.6" "List executions" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "30.6" "List executions" "skip" "no workflow"
fi

# ── 30.7 Create workflow with empty name (400) ─────────────────────────────
log_test_start "30.7" "POST /workflows with empty name returns 400"
pepa_api POST "/workflows" '{"name": "", "spec": {}}'
if [[ "$API_STATUS" == "400" ]]; then
    log_test "30.7" "Empty name returns 400" "pass"
else
    log_test "30.7" "Empty name" "fail" "expected 400, got $API_STATUS"
fi

# ── 30.8 Get non-existent workflow (404) ───────────────────────────────────
log_test_start "30.8" "GET /workflows/00000000-...-0099 returns 404"
pepa_api GET "/workflows/00000000-0000-0000-0000-000000000099"
if [[ "$API_STATUS" == "404" ]]; then
    log_test "30.8" "Non-existent workflow returns 404" "pass"
else
    log_test "30.8" "Non-existent workflow" "fail" "expected 404, got $API_STATUS"
fi

# ── 30.9 Delete workflow ───────────────────────────────────────────────────
if [[ -n "$WF_ID" ]]; then
    log_test_start "30.9" "DELETE /workflows/:id"
    pepa_api DELETE "/workflows/$WF_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "30.9" "Workflow deleted" "pass"
    else
        log_test "30.9" "Delete workflow" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "30.9" "Delete workflow" "skip" "no workflow"
fi

# ── 30.10 Verify deletion (404) ────────────────────────────────────────────
if [[ -n "$WF_ID" ]]; then
    log_test_start "30.10" "GET /workflows/:id returns 404 after deletion"
    pepa_api GET "/workflows/$WF_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "30.10" "Deleted workflow returns 404" "pass"
    else
        log_test "30.10" "Deleted workflow" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "30.10" "Verify deletion" "skip" "no workflow"
fi

print_summary
