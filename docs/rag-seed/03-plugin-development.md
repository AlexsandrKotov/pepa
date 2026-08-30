# PEPA — Plugin System

## Overview

PEPA uses a **plugin-based architecture** where every external integration is a plugin. Plugins are provided by the PEPA team and can be installed, enabled, or disabled at runtime without restarting the platform.

## Available Plugins

| Plugin | Type | Purpose |
|--------|------|---------|
| `gitlab` | Git | GitLab integration (repos, MRs, CI/CD) |
| `github` | Git | GitHub integration (repos, PRs, Actions) |
| `gitea` | Git | Gitea integration |
| `argocd` | Deploy | ArgoCD deployment sync |
| `slack` | Notification | Slack notifications |
| `telegram` | Notification | Telegram notifications |
| `email` | Notification | Email notifications |
| `jira` | Task | Jira issue tracking |
| `trivy` | Security | Vulnerability scanning |
| `webhook` | Integration | Generic webhook handler |

## Installing Plugins

Plugins are installed via the **Marketplace** page:

1. Go to **Marketplace** in the sidebar
2. Find the plugin you need
3. Click **Install**
4. Configure the plugin via **Connections** page

## Configuring Plugins

All plugin configuration is done via the **Connections** page:

1. Go to **Connections** → **Add Connection**
2. Select the plugin type
3. Enter credentials (API token, URL, etc.)
4. Click **Save**

Credentials are stored encrypted. Vault references (`vault:path/key`) are resolved at runtime.

### Example: GitLab Plugin

1. Install `gitlab` plugin from Marketplace
2. Go to Connections → Add Connection → GitLab
3. Enter:
   - Base URL: `https://gitlab.com` (or your self-hosted URL)
   - Access Token: your GitLab personal access token
4. Save

### Example: Slack Plugin

1. Install `slack` plugin from Marketplace
2. Go to Connections → Add Connection → Slack
3. Enter:
   - Bot Token: `xoxb-...`
   - Channel: `#deployments`
4. Save

## Plugin Actions

Each plugin provides actions that can be used in workflows:

| Plugin | Actions |
|--------|---------|
| `gitlab` | `list_repos`, `create_mr`, `merge_mr`, `trigger_pipeline`, `get_pipeline_status` |
| `github` | `list_repos`, `create_pr`, `merge_pr`, `trigger_workflow`, `get_workflow_status` |
| `argocd` | `sync_app`, `get_app_status`, `list_apps`, `rollback` |
| `slack` | `send_message`, `send_alert` |
| `jira` | `create_issue`, `update_issue`, `list_issues`, `transition_issue` |
| `trivy` | `scan_image`, `scan_fs`, `get_report` |

## Using Plugins in Workflows

Plugins are used as steps in workflows:

```yaml
name: Deploy to Production
trigger: manual
steps:
  - name: Scan for vulnerabilities
    plugin: trivy
    action: scan_image
    params:
      image: "myapp:latest"
      
  - name: Sync to ArgoCD
    plugin: argocd
    action: sync_app
    params:
      app: myapp-prod
      
  - name: Notify Slack
    plugin: slack
    action: send_message
    params:
      channel: "#deployments"
      message: "Deployed myapp to production"
    depends_on: [Scan for vulnerabilities, Sync to ArgoCD]
```

## Troubleshooting

**Plugin not showing in Marketplace**: Check that the plugin binary exists in `plugins/bin/`. Restart the API server.

**Plugin action fails**: Check the plugin logs in `docker compose logs pepa-api | grep <plugin-name>`. Verify credentials in Connections page.

**Connection test fails**: Verify the API token/URL is correct. Check network connectivity from the PEPA container to the external service.
