#!/usr/bin/env bash
# 31-test-pipeline-sources.sh — Pipeline sources CRUD, runs, presets
# Covers: POST /pipeline-sources, GET /pipeline-sources, GET /pipeline-sources/:id,
#         PUT /pipeline-sources/:id, DELETE /pipeline-sources/:id,
#         GET /pipeline-sources/:id/state, GET /pipeline-sources/stats,
#         POST /pipeline-sources/:id/runs, GET .../runs/:runId

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"
source "$SCRIPT_DIR/lib/assertions.sh"

log_phase "Phase 31: Pipeline Sources, Runs & Presets"

pepa_login 2>/dev/null || true

# ── 31.1 List pipeline sources ─────────────────────────────────────────────
log_test_start "31.1" "GET /pipeline-sources returns source list"
pepa_api GET "/pipeline-sources"
if [[ "$API_STATUS" == "200" ]]; then
    SRC_COUNT=$(echo "$API_RESPONSE" | jq '.sources // .items // [] | length' 2>/dev/null)
    log_test "31.1" "Pipeline sources: $SRC_COUNT found" "pass"
else
    log_test "31.1" "List pipeline sources" "fail" "HTTP $API_STATUS"
fi

# ── 31.2 Get pipeline stats ────────────────────────────────────────────────
log_test_start "31.2" "GET /pipeline-sources/stats returns engine stats"
pepa_api GET "/pipeline-sources/stats"
if [[ "$API_STATUS" == "200" ]]; then
    log_test "31.2" "Pipeline stats endpoint accessible" "pass"
else
    log_test "31.2" "Pipeline stats" "fail" "HTTP $API_STATUS"
fi

# ── 31.3 Create pipeline source ────────────────────────────────────────────
log_test_start "31.3" "POST /pipeline-sources creates a new source"
SRC_BODY='{
    "name": "test-pipeline-source",
    "source_type": "gitlab",
    "repo_url": "http://localhost:3001/pepa/test-repo.git",
    "branch": "main",
    "config": {"pipeline_file": ".gitlab-ci.yml"}
}'
pepa_api POST "/pipeline-sources" "$SRC_BODY"
if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
    SRC_ID=$(echo "$API_RESPONSE" | jq -r '.id // .source.id // empty')
    log_test "31.3" "Pipeline source created: ${SRC_ID:0:8}..." "pass"
else
    log_test "31.3" "Create pipeline source" "fail" "HTTP $API_STATUS"
    SRC_ID=""
fi

# ── 31.4 Get pipeline source by ID ─────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.4" "GET /pipeline-sources/:id returns source details"
    pepa_api GET "/pipeline-sources/$SRC_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        SRC_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        SRC_TYPE=$(echo "$API_RESPONSE" | jq -r '.source_type // empty')
        log_test "31.4" "Source: name=$SRC_NAME, type=$SRC_TYPE" "pass"
    else
        log_test "31.4" "Get pipeline source" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.4" "Get pipeline source" "skip" "no source"
fi

# ── 31.5 Update pipeline source ────────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.5" "PUT /pipeline-sources/:id updates source"
    pepa_api PUT "/pipeline-sources/$SRC_ID" '{"name": "test-pipeline-source-updated", "branch": "develop", "source_type": "gitlab", "repo_url": "http://localhost:3001/pepa/test-repo.git"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        UPD_NAME=$(echo "$API_RESPONSE" | jq -r '.name // empty')
        log_test "31.5" "Source updated: name=$UPD_NAME" "pass"
    else
        log_test "31.5" "Update pipeline source" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.5" "Update pipeline source" "skip" "no source"
fi

# ── 31.6 Get pipeline source state ─────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.6" "GET /pipeline-sources/:id/state returns source state"
    pepa_api GET "/pipeline-sources/$SRC_ID/state"
    if [[ "$API_STATUS" == "200" || "$API_STATUS" == "400" ]]; then
        log_test "31.6" "Pipeline source state endpoint accessible (HTTP $API_STATUS)" "pass"
    else
        log_test "31.6" "Pipeline source state" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.6" "Pipeline source state" "skip" "no source"
fi

# ── 31.7 Inspect pipeline source ───────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.7" "GET /pipeline-sources/:id/inspect inspects source"
    pepa_api GET "/pipeline-sources/$SRC_ID/inspect"
    if [[ "$API_STATUS" == "200" || "$API_STATUS" == "400" ]]; then
        log_test "31.7" "Pipeline source inspect endpoint accessible (HTTP $API_STATUS)" "pass"
    else
        log_test "31.7" "Pipeline source inspect" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.7" "Pipeline source inspect" "skip" "no source"
fi

# ── 31.8 List pipeline runs ────────────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.8" "GET /pipeline-sources/:id/runs lists runs"
    pepa_api GET "/pipeline-sources/$SRC_ID/runs"
    if [[ "$API_STATUS" == "200" ]]; then
        RUN_COUNT=$(echo "$API_RESPONSE" | jq '.runs // .items // [] | length' 2>/dev/null)
        log_test "31.8" "Pipeline runs: $RUN_COUNT found" "pass"
    else
        log_test "31.8" "List pipeline runs" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.8" "List pipeline runs" "skip" "no source"
fi

# ── 31.9 Trigger pipeline run ──────────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.9" "POST /pipeline-sources/:id/runs triggers a run"
    pepa_api POST "/pipeline-sources/$SRC_ID/runs" '{"branch": "main", "ref": "abc123"}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        RUN_ID=$(echo "$API_RESPONSE" | jq -r '.id // .run_id // empty')
        RUN_STATUS=$(echo "$API_RESPONSE" | jq -r '.status // empty')
        log_test "31.9" "Run triggered: id=${RUN_ID:0:8}..., status=$RUN_STATUS" "pass"
    else
        log_test "31.9" "Trigger pipeline run" "fail" "HTTP $API_STATUS"
        RUN_ID=""
    fi
else
    log_test "31.9" "Trigger pipeline run" "skip" "no source"
    RUN_ID=""
fi

# ── 31.10 Get pipeline run details ─────────────────────────────────────────
if [[ -n "${SRC_ID:-}" && -n "${RUN_ID:-}" ]]; then
    log_test_start "31.10" "GET /pipeline-sources/:id/runs/:runId returns run details"
    pepa_api GET "/pipeline-sources/$SRC_ID/runs/$RUN_ID"
    if [[ "$API_STATUS" == "200" ]]; then
        log_test "31.10" "Pipeline run details accessible" "pass"
    else
        log_test "31.10" "Get pipeline run" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.10" "Get pipeline run" "skip" "no run"
fi

# ── 31.11 Get pipeline run jobs ────────────────────────────────────────────
if [[ -n "${SRC_ID:-}" && -n "${RUN_ID:-}" ]]; then
    log_test_start "31.11" "GET /pipeline-sources/:id/runs/:runId/jobs lists jobs"
    pepa_api GET "/pipeline-sources/$SRC_ID/runs/$RUN_ID/jobs"
    if [[ "$API_STATUS" == "200" ]]; then
        JOB_COUNT=$(echo "$API_RESPONSE" | jq '.jobs // .items // [] | length' 2>/dev/null)
        log_test "31.11" "Pipeline run jobs: $JOB_COUNT found" "pass"
    else
        log_test "31.11" "List pipeline run jobs" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.11" "List pipeline run jobs" "skip" "no run"
fi

# ── 31.12 List pipeline presets ────────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.12" "GET /pipeline-sources/:id/presets lists presets"
    pepa_api GET "/pipeline-sources/$SRC_ID/presets"
    if [[ "$API_STATUS" == "200" ]]; then
        PRE_COUNT=$(echo "$API_RESPONSE" | jq '.presets // .items // [] | length' 2>/dev/null)
        log_test "31.12" "Pipeline presets: $PRE_COUNT found" "pass"
    else
        log_test "31.12" "List pipeline presets" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.12" "List pipeline presets" "skip" "no source"
fi

# ── 31.13 Create pipeline preset ───────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.13" "POST /pipeline-sources/:id/presets creates preset"
    pepa_api POST "/pipeline-sources/$SRC_ID/presets" '{"name": "test-preset", "variables": {"ENV": "staging"}}'
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        PRE_ID=$(echo "$API_RESPONSE" | jq -r '.id // .preset.id // empty')
        log_test "31.13" "Preset created: ${PRE_ID:0:8}..." "pass"
    else
        log_test "31.13" "Create preset" "fail" "HTTP $API_STATUS"
        PRE_ID=""
    fi
else
    log_test "31.13" "Create preset" "skip" "no source"
    PRE_ID=""
fi

# ── 31.14 Delete pipeline source ───────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.14" "DELETE /pipeline-sources/:id"
    pepa_api DELETE "/pipeline-sources/$SRC_ID"
    if [[ "$API_STATUS" =~ ^2[0-9][0-9]$ ]]; then
        log_test "31.14" "Pipeline source deleted" "pass"
    else
        log_test "31.14" "Delete pipeline source" "fail" "HTTP $API_STATUS"
    fi
else
    log_test "31.14" "Delete pipeline source" "skip" "no source"
fi

# ── 31.15 Verify deletion (404) ────────────────────────────────────────────
if [[ -n "$SRC_ID" ]]; then
    log_test_start "31.15" "GET /pipeline-sources/:id returns 404 after deletion"
    pepa_api GET "/pipeline-sources/$SRC_ID"
    if [[ "$API_STATUS" == "404" ]]; then
        log_test "31.15" "Deleted source returns 404" "pass"
    else
        log_test "31.15" "Deleted source" "fail" "expected 404, got $API_STATUS"
    fi
else
    log_test "31.15" "Verify deletion" "skip" "no source"
fi

print_summary
