package provider

// Plugin name constants. These replace the ~250 string literals scattered
// across handlers, services, and dispatch tables. Using constants ensures
// compile-time safety and makes renaming a plugin a single-line change.
const (
	PluginGitLab        = "gitlab"
	PluginGitHub        = "github"
	PluginGitea         = "gitea"
	PluginBitbucket     = "bitbucket"
	PluginJenkins       = "jenkins"
	PluginJira          = "jira"
	PluginArgoCD        = "argocd"
	PluginFluxCD        = "fluxcd"
	PluginKubernetes    = "kubernetes"
	PluginTrivy         = "trivy"
	PluginSonarQube     = "sonarqube"
	PluginDocker        = "docker"
	PluginProxmox       = "proxmox"
	PluginVMware        = "vmware"
	PluginSlack         = "slack"
	PluginTelegram      = "telegram"
	PluginTeams         = "teams"
	PluginS3            = "s3"
	PluginWebhook       = "webhook"
	PluginEmail         = "email"
	PluginRemoteConsole = "remote-console"
	PluginAIBot         = "ai_bot"
)
