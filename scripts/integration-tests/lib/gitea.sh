#!/usr/bin/env bash
# lib/gitea.sh — Gitea helpers for PEPA integration tests
# Source this file after common.sh

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

: "${GITEA_URL:=http://localhost:3001}"
: "${GITEA_ADMIN_USER:=pepa}"
: "${GITEA_ADMIN_PASS:=PepaTest2026!}"
: "${GITEA_ORG:=pepa-test}"
: "${GITEA_TOKEN_FILE:=/tmp/gitea_token}"

# Internal token cache
_GITEA_TOKEN=""
_GITEA_API=""

# ---------------------------------------------------------------------------
# Token management
# ---------------------------------------------------------------------------

gitea_init() {
    _GITEA_API="${GITEA_URL}/api/v1"
    # Try to read existing token file
    if [[ -f "$GITEA_TOKEN_FILE" ]]; then
        _GITEA_TOKEN=$(cat "$GITEA_TOKEN_FILE")
        log_info "Gitea token loaded from $GITEA_TOKEN_FILE"
        return 0
    fi
    # Try to create a personal access token via API
    log_info "Creating Gitea API token..."
    local resp
    resp=$(curl -s -w "\n%{http_code}" -X POST "${_GITEA_API}/users/${GITEA_ADMIN_USER}/tokens" \
        -u "${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}" \
        -H "Content-Type: application/json" \
        -d "{\"name\":\"pepa-integration-test-$(date +%s)\"}" 2>/dev/null)
    local code body
    code=$(echo "$resp" | tail -1)
    body=$(echo "$resp" | sed '$d')
    if [[ "$code" == "201" ]]; then
        _GITEA_TOKEN=$(echo "$body" | jq -r '.sha1 // .token // empty')
        echo "$_GITEA_TOKEN" > "$GITEA_TOKEN_FILE"
        log_info "Gitea token created and saved to $GITEA_TOKEN_FILE"
    else
        log_warn "Could not create Gitea token (HTTP $code). Using basic auth."
        _GITEA_TOKEN=""
    fi
}

gitea_get_token() {
    if [[ -z "$_GITEA_TOKEN" ]]; then
        gitea_init
    fi
    echo "$_GITEA_TOKEN"
}

# ---------------------------------------------------------------------------
# Internal curl wrapper
# ---------------------------------------------------------------------------

_gitea_curl() {
    local method="$1" url="$2" data="${3:-}"
    local args=(-s -w "\n%{http_code}" -X "$method")
    if [[ -n "$_GITEA_TOKEN" ]]; then
        args+=(-H "Authorization: token $_GITEA_TOKEN")
    else
        args+=(-u "${GITEA_ADMIN_USER}:${GITEA_ADMIN_PASS}")
    fi
    args+=(-H "Content-Type: application/json")
    if [[ -n "$data" ]]; then
        args+=(-d "$data")
    fi
    args+=("${_GITEA_API}${url}")
    curl "${args[@]}"
}

# ---------------------------------------------------------------------------
# Organization management
# ---------------------------------------------------------------------------

gitea_ensure_org() {
    local org="$1"
    local resp code body
    # Check if org exists
    resp=$(_gitea_curl GET "/orgs/${org}")
    code=$(echo "$resp" | tail -1)
    if [[ "$code" == "200" ]]; then
        log_info "Gitea org '$org' already exists"
        return 0
    fi
    # Create org
    log_info "Creating Gitea org '$org'..."
    resp=$(_gitea_curl POST "/orgs" "{\"username\":\"${org}\",\"full_name\":\"${org} Integration Test Org\",\"visibility\":\"public\"}")
    code=$(echo "$resp" | tail -1)
    body=$(echo "$resp" | sed '$d')
    if [[ "$code" == "201" ]]; then
        log_info "Gitea org '$org' created"
        return 0
    fi
    log_warn "Could not create org '$org' (HTTP $code): ${body:0:200}"
    return 1
}

# ---------------------------------------------------------------------------
# Repository management
# ---------------------------------------------------------------------------

# gitea_create_repo <repo_name> [description] [auto_init]
gitea_create_repo() {
    local name="$1" desc="${2:-Integration test repo}" auto_init="${3:-true}"
    local resp code body
    # Check if repo exists
    resp=$(_gitea_curl GET "/repos/${GITEA_ORG}/${name}")
    code=$(echo "$resp" | tail -1)
    if [[ "$code" == "200" ]]; then
        log_info "Gitea repo '${GITEA_ORG}/${name}' already exists"
        return 0
    fi
    log_info "Creating Gitea repo '${GITEA_ORG}/${name}'..."
    resp=$(_gitea_curl POST "/orgs/${GITEA_ORG}/repos" \
        "{\"name\":\"${name}\",\"description\":\"${desc}\",\"auto_init\":${auto_init},\"default_branch\":\"main\",\"visibility\":\"public\"}")
    code=$(echo "$resp" | tail -1)
    body=$(echo "$resp" | sed '$d')
    if [[ "$code" == "201" ]]; then
        log_info "Repo '${GITEA_ORG}/${name}' created"
        return 0
    fi
    log_error "Failed to create repo '${GITEA_ORG}/${name}' (HTTP $code): ${body:0:200}"
    return 1
}

# gitea_delete_repo <repo_name>
gitea_delete_repo() {
    local name="$1"
    local resp code
    resp=$(_gitea_curl DELETE "/repos/${GITEA_ORG}/${name}")
    code=$(echo "$resp" | tail -1)
    if [[ "$code" == "204" || "$code" == "200" ]]; then
        log_info "Repo '${GITEA_ORG}/${name}' deleted"
        return 0
    fi
    log_warn "Could not delete repo '${GITEA_ORG}/${name}' (HTTP $code)"
    return 1
}

# ---------------------------------------------------------------------------
# File / content management
# ---------------------------------------------------------------------------

# gitea_create_file <repo_name> <path> <content> <commit_message> [branch]
gitea_create_file() {
    local repo="$1" path="$2" content="$3" message="${4:-Add file}" branch="${5:-main}"
    local encoded
    encoded=$(echo -n "$content" | base64 | tr -d '\n')
    local resp code body
    resp=$(_gitea_curl POST "/repos/${GITEA_ORG}/${repo}/contents/${path}" \
        "{\"message\":\"${message}\",\"content\":\"${encoded}\",\"branch\":\"${branch}\"}")
    code=$(echo "$resp" | tail -1)
    body=$(echo "$resp" | sed '$d')
    if [[ "$code" == "201" ]]; then
        log_info "File '${path}' created in ${GITEA_ORG}/${repo}"
        return 0
    fi
    log_error "Failed to create file '${path}' (HTTP $code): ${body:0:200}"
    return 1
}

# gitea_update_file <repo_name> <path> <content> <commit_message> [branch]
gitea_update_file() {
    local repo="$1" path="$2" content="$3" message="${4:-Update file}" branch="${5:-main}"
    local encoded sha
    encoded=$(echo -n "$content" | base64 | tr -d '\n')
    # Get current file SHA
    sha=$(curl -s -H "Authorization: token $_GITEA_TOKEN" \
        "${_GITEA_API}/repos/${GITEA_ORG}/${repo}/contents/${path}?ref=${branch}" \
        | jq -r '.sha // empty' 2>/dev/null)
    if [[ -z "$sha" ]]; then
        log_error "Cannot get SHA for '${path}' in ${GITEA_ORG}/${repo}"
        return 1
    fi
    local resp code body
    resp=$(_gitea_curl PUT "/repos/${GITEA_ORG}/${repo}/contents/${path}" \
        "{\"message\":\"${message}\",\"content\":\"${encoded}\",\"sha\":\"${sha}\",\"branch\":\"${branch}\"}")
    code=$(echo "$resp" | tail -1)
    body=$(echo "$resp" | sed '$d')
    if [[ "$code" == "200" ]]; then
        log_info "File '${path}' updated in ${GITEA_ORG}/${repo}"
        return 0
    fi
    log_error "Failed to update file '${path}' (HTTP $code): ${body:0:200}"
    return 1
}

# gitea_delete_file <repo_name> <path> <commit_message> [branch]
gitea_delete_file() {
    local repo="$1" path="$2" message="${3:-Delete file}" branch="${4:-main}"
    local sha
    sha=$(curl -s -H "Authorization: token $_GITEA_TOKEN" \
        "${_GITEA_API}/repos/${GITEA_ORG}/${repo}/contents/${path}?ref=${branch}" \
        | jq -r '.sha // empty' 2>/dev/null)
    if [[ -z "$sha" ]]; then
        log_warn "File '${path}' not found in ${GITEA_ORG}/${repo}"
        return 0
    fi
    local resp code
    resp=$(_gitea_curl DELETE "/repos/${GITEA_ORG}/${repo}/contents/${path}" \
        "{\"message\":\"${message}\",\"sha\":\"${sha}\",\"branch\":\"${branch}\"}")
    code=$(echo "$resp" | tail -1)
    if [[ "$code" == "200" || "$code" == "204" ]]; then
        log_info "File '${path}' deleted from ${GITEA_ORG}/${repo}"
        return 0
    fi
    log_warn "Could not delete file '${path}' (HTTP $code)"
    return 1
}

# ---------------------------------------------------------------------------
# Push manifest directory to repo
# ---------------------------------------------------------------------------

# gitea_push_manifest <repo_name> <local_dir> [commit_message] [branch]
#   Pushes all files from local_dir to the repo using Gitea API
gitea_push_manifest() {
    local repo="$1" local_dir="$2" message="${3:-Push manifests}" branch="${4:-main}"
    local count=0 errors=0
    while IFS= read -r -d '' file; do
        local rel_path="${file#${local_dir}/}"
        local content
        content=$(cat "$file")
        # Try update first (file may exist), then create
        if ! gitea_update_file "$repo" "$rel_path" "$content" "$message" "$branch" 2>/dev/null; then
            if ! gitea_create_file "$repo" "$rel_path" "$content" "$message" "$branch" 2>/dev/null; then
                ((errors++))
            fi
        fi
        ((count++))
    done < <(find "$local_dir" -type f -print0 2>/dev/null)
    log_info "Pushed $count files to ${GITEA_ORG}/${repo} ($errors errors)"
    return $errors
}

# gitea_push_manifest_via_git <repo_name> <local_dir> [commit_message] [branch]
#   Uses git clone + push (requires git CLI and credentials)
gitea_push_manifest_via_git() {
    local repo="$1" local_dir="$2" message="${3:-Push manifests}" branch="${4:-main}"
    local tmp_dir="${RESULTS_DIR}/tmp-git-${repo}-$$"
    local repo_url
    if [[ -n "$_GITEA_TOKEN" ]]; then
        repo_url="${GITEA_URL}/${GITEA_ORG}/${repo}.git"
        # Use token in URL for auth
        repo_url="${GITEA_URL%%://*}://${GITEA_ADMIN_USER}:${_GITEA_TOKEN}@${GITEA_URL#*://}${GITEA_URL#*:*}/${GITEA_ORG}/${repo}.git"
        # Reconstruct properly
        local proto="${GITEA_URL%%://*}"
        local host_port="${GITEA_URL#*://}"
        repo_url="${proto}://${GITEA_ADMIN_USER}:${_GITEA_TOKEN}@${host_port}/${GITEA_ORG}/${repo}.git"
    else
        repo_url="${GITEA_URL}/${GITEA_ORG}/${repo}.git"
    fi

    mkdir -p "$tmp_dir"
    git clone --branch "$branch" "$repo_url" "$tmp_dir" 2>/dev/null || \
    git clone "$repo_url" "$tmp_dir" 2>/dev/null || {
        log_error "Failed to clone ${GITEA_ORG}/${repo}"
        return 1
    }

    # Copy files
    cp -r "$local_dir"/* "$tmp_dir"/ 2>/dev/null
    cd "$tmp_dir" || return 1
    git config user.email "pepa-test@local"
    git config user.name "PEPA Integration Test"
    git add -A
    if git diff --cached --quiet 2>/dev/null; then
        log_info "No changes to push for ${GITEA_ORG}/${repo}"
        rm -rf "$tmp_dir"
        return 0
    fi
    git commit -m "$message"
    git push origin HEAD:"$branch" 2>/dev/null || git push 2>/dev/null
    local rc=$?
    cd - >/dev/null
    rm -rf "$tmp_dir"
    if [[ $rc -eq 0 ]]; then
        log_info "Manifests pushed to ${GITEA_ORG}/${repo}"
    else
        log_error "Git push failed for ${GITEA_ORG}/${repo}"
    fi
    return $rc
}

# ---------------------------------------------------------------------------
# Branch management
# ---------------------------------------------------------------------------

# gitea_create_branch <repo_name> <branch_name> [from_branch]
gitea_create_branch() {
    local repo="$1" branch="$2" from="${3:-main}"
    local resp code
    resp=$(_gitea_curl POST "/repos/${GITEA_ORG}/${repo}/branches" \
        "{\"branch_name\":\"${branch}\",\"old_branch_name\":\"${from}\"}")
    code=$(echo "$resp" | tail -1)
    if [[ "$code" == "201" ]]; then
        log_info "Branch '$branch' created in ${GITEA_ORG}/${repo}"
        return 0
    fi
    log_warn "Could not create branch '$branch' (HTTP $code)"
    return 1
}

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

# gitea_cleanup_repos [repo1 repo2 ...]  — delete specified repos or all pepa-test repos
gitea_cleanup_repos() {
    local repos=("$@")
    if [[ ${#repos[@]} -eq 0 ]]; then
        # List all repos in org and delete them
        local list
        list=$(curl -s -H "Authorization: token $_GITEA_TOKEN" \
            "${_GITEA_API}/orgs/${GITEA_ORG}/repos?limit=50" 2>/dev/null \
            | jq -r '.[].name' 2>/dev/null)
        repos=($list)
    fi
    local deleted=0
    for repo in "${repos[@]}"; do
        if gitea_delete_repo "$repo" 2>/dev/null; then
            ((deleted++))
        fi
    done
    log_info "Cleaned up $deleted Gitea repos"
}

# ---------------------------------------------------------------------------
# Utility: get repo clone URL
# ---------------------------------------------------------------------------

gitea_clone_url() {
    local repo="$1"
    echo "${GITEA_URL}/${GITEA_ORG}/${repo}.git"
}

# gitea_repo_api_url <repo_name>
gitea_repo_api_url() {
    local repo="$1"
    echo "${GITEA_URL}/api/v1/repos/${GITEA_ORG}/${repo}"
}
