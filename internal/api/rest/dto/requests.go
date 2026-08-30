// Package dto provides Data Transfer Objects for API request/response validation.
package dto

// CreateServiceRequest represents the request body for creating a service.
type CreateServiceRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	TypeKey     string `json:"type_key" binding:"required,max=100"`
	Description string `json:"description" binding:"max=2000"`
	Owner       string `json:"owner" binding:"max=255"`
	Team        string `json:"team" binding:"max=255"`
	URL         string `json:"url" binding:"max=500"`
	Repository  string `json:"repository" binding:"max=500"`
}

// UpdateServiceRequest represents the request body for updating a service.
type UpdateServiceRequest struct {
	Name        string `json:"name" binding:"max=255"`
	TypeKey     string `json:"type_key" binding:"max=100"`
	Description string `json:"description" binding:"max=2000"`
	Owner       string `json:"owner" binding:"max=255"`
	Team        string `json:"team" binding:"max=255"`
	URL         string `json:"url" binding:"max=500"`
	Repository  string `json:"repository" binding:"max=500"`
}

// CreatePipelineRequest represents the request body for creating a pipeline.
type CreatePipelineRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=2000"`
	SourceID    string `json:"source_id" binding:"required,max=100"`
	Branch      string `json:"branch" binding:"max=255"`
	ConfigPath  string `json:"config_path" binding:"max=500"`
}

// CreateWorkflowRequest represents the request body for creating a workflow.
type CreateWorkflowRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=2000"`
	Template    string `json:"template" binding:"max=100"`
	TeamID      string `json:"team_id" binding:"max=100"`
}

// CreateClusterRequest represents the request body for creating a cluster.
type CreateClusterRequest struct {
	Name       string `json:"name" binding:"required,min=1,max=255"`
	Type       string `json:"type" binding:"required,max=50"`
	APIURL     string `json:"api_url" binding:"required,max=500"`
	Kubeconfig string `json:"kubeconfig" binding:"max=10000"`
	Namespace  string `json:"namespace" binding:"max=255"`
}

// CreateConnectionRequest represents the request body for creating a connection.
type CreateConnectionRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Type        string `json:"type" binding:"required,max=100"`
	URL         string `json:"url" binding:"required,max=500"`
	Token       string `json:"token" binding:"max=2000"`
	Username    string `json:"username" binding:"max=255"`
	Password    string `json:"password" binding:"max=2000"`
	Description string `json:"description" binding:"max=2000"`
}

// CreateEnvironmentRequest represents the request body for creating an environment.
type CreateEnvironmentRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=2000"`
	ClusterID   string `json:"cluster_id" binding:"max=100"`
	Namespace   string `json:"namespace" binding:"max=255"`
	Stage       string `json:"stage" binding:"max=100"`
}

// CreateUserRequest represents the request body for creating a user.
type CreateUserRequest struct {
	Email       string   `json:"email" binding:"required,min=5,max=255"`
	Name        string   `json:"name" binding:"required,min=1,max=255"`
	Password    string   `json:"password" binding:"required,min=8,max=255"`
	Roles       []string `json:"roles" binding:"max=10"`
	DisplayName string   `json:"display_name" binding:"max=255"`
}

// UpdateUserRequest represents the request body for updating a user.
type UpdateUserRequest struct {
	Email       string   `json:"email" binding:"max=255"`
	Name        string   `json:"name" binding:"max=255"`
	DisplayName string   `json:"display_name" binding:"max=255"`
	Roles       []string `json:"roles" binding:"max=10"`
	IsActive    *bool    `json:"is_active"`
}

// LoginRequest represents the request body for user login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,min=5,max=255"`
	Password string `json:"password" binding:"required,min=1,max=255"`
}

// CreateGitopsRepoRequest represents the request body for creating a GitOps repository.
type CreateGitopsRepoRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	URL         string `json:"url" binding:"required,max=500"`
	Branch      string `json:"branch" binding:"max=255"`
	Path        string `json:"path" binding:"max=500"`
	Token       string `json:"token" binding:"max=2000"`
	EngineType  string `json:"engine_type" binding:"max=100"`
	ClusterID   string `json:"cluster_id" binding:"max=100"`
	Description string `json:"description" binding:"max=2000"`
}

// CreateScorecardRequest represents the request body for creating a scorecard.
type CreateScorecardRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=2000"`
	Category    string `json:"category" binding:"max=100"`
	Weight      int    `json:"weight" binding:"min=1,max=100"`
}

// CreateVaultRequest represents the request body for creating a vault entry.
type CreateVaultRequest struct {
	Path      string            `json:"path" binding:"required,min=1,max=500"`
	Data      map[string]string `json:"data" binding:"required"`
	Namespace string            `json:"namespace" binding:"max=255"`
}

// PaginationRequest represents common pagination parameters.
type PaginationRequest struct {
	Page   int `json:"page" form:"page" binding:"min=1"`
	Limit  int `json:"limit" form:"limit" binding:"min=1,max=200"`
	Offset int `json:"offset" form:"offset" binding:"min=0"`
}

// CursorPaginationRequest represents cursor-based pagination parameters.
type CursorPaginationRequest struct {
	Cursor string `json:"cursor" form:"cursor" binding:"max=500"`
	Limit  int    `json:"limit" form:"limit" binding:"min=1,max=200"`
}
