# Frontend API Module Split Plan

## Overview
Split the monolithic `lib/api.ts` (3601 lines) into logical, domain-specific modules for better maintainability.

## Current State
- **File:** `lib/api.ts`
- **Lines:** 3601
- **Functions:** 300+
- **Issues:** Hard to navigate, difficult to test, large bundle impact

## Target Structure

```
lib/
├── api.ts (legacy - will be deprecated)
└── api/
    ├── index.ts (re-exports all modules)
    ├── core.ts (utilities, auth helpers)
    ├── auth.ts (login, logout, OIDC)
    ├── users.ts (user management)
    ├── teams.ts (team operations)
    ├── credentials.ts (credential management)
    ├── entities.ts (entity CRUD)
    ├── services.ts (service operations)
    ├── pipelines.ts (pipeline management)
    ├── workflows.ts (workflow operations)
    ├── clusters.ts (cluster management)
    ├── connections.ts (connection config)
    ├── environments.ts (environment ops)
    ├── plugins.ts (plugin management)
    ├── marketplace.ts (marketplace ops)
    ├── vault.ts (secret management)
    ├── roles.ts (RBAC operations)
    ├── gitops.ts (GitOps operations)
    ├── discovery.ts (service discovery)
    ├── audit.ts (audit logs)
    ├── settings.ts (system settings)
    ├── scorecards.ts (scorecard ops)
    ├── ai.ts (AI chat, providers)
    ├── virtualization.ts (VM, containers)
    ├── health.ts (health checks)
    ├── docker.ts (Docker operations)
    ├── s3.ts (S3 browser)
    ├── jira.ts (Jira integration)
    └── helm.ts (Helm repositories)
```

## Module Breakdown

### Core Module (~100 lines)
```typescript
// lib/api/core.ts
export function getBase(): string { ... }
export function getToken(): string | null { ... }
export function setToken(token: string): void { ... }
export function removeToken(): void { ... }
export function getStoredUser(): User | null { ... }
export function setStoredUser(user: User): void { ... }
export function isAuthenticated(): boolean { ... }
export function authHeaders(): Record<string, string> { ... }
```

### Auth Module (~200 lines)
```typescript
// lib/api/auth.ts
export async function login(email, password): Promise<LoginResponse> { ... }
export async function logout(): Promise<void> { ... }
export async function getOIDCConfig(): Promise<OIDCConfig> { ... }
export async function getOIDCLoginURL(): Promise<LoginURL> { ... }
export async function getMe(): Promise<UserInfo> { ... }
export async function refreshMe(): Promise<TokenInfo> { ... }
export async function resetMyPassword(current, new): Promise<UserInfo> { ... }
export async function getBootstrapStatus(): Promise<BootstrapStatus> { ... }
export async function bootstrapActivate(token, password): Promise<LoginResponse> { ... }
```

### Users Module (~150 lines)
```typescript
// lib/api/users.ts
export async function listUsers(search?): Promise<UserList> { ... }
export async function createUser(data): Promise<UserId> { ... }
export async function getUser(id): Promise<UserInfo> { ... }
export async function updateUser(id, data): Promise<void> { ... }
export async function deactivateUser(id): Promise<void> { ... }
export async function resetUserPassword(id, password): Promise<void> { ... }
```

### Teams Module (~200 lines)
```typescript
// lib/api/teams.ts
export async function listTeams(): Promise<TeamList> { ... }
export async function createTeam(data): Promise<TeamId> { ... }
export async function getTeam(id): Promise<TeamInfo> { ... }
export async function updateTeam(id, data): Promise<void> { ... }
export async function deleteTeam(id): Promise<void> { ... }
export async function listTeamMembers(teamId): Promise<MemberList> { ... }
export async function addTeamMember(teamId, userId, role?): Promise<void> { ... }
export async function removeTeamMember(teamId, userId): Promise<void> { ... }
export async function getTeamRoles(teamId): Promise<RoleList> { ... }
export async function assignTeamRole(teamId, roleId): Promise<void> { ... }
export async function removeTeamRole(teamId, roleId): Promise<void> { ... }
```

### Credentials Module (~250 lines)
```typescript
// lib/api/credentials.ts
export async function listMyCredentials(): Promise<CredentialList> { ... }
export async function createMyCredential(data): Promise<CredentialId> { ... }
export async function updateMyCredential(id, data): Promise<void> { ... }
export async function deleteMyCredential(id): Promise<void> { ... }
export async function verifyMyCredential(id): Promise<VerifyResult> { ... }
export async function fetchUserInfoForCredential(...): Promise<UserInfo> { ... }
export async function listSharedCredentials(): Promise<CredentialList> { ... }
export async function shareCredential(id, data): Promise<ShareId> { ... }
export async function listCredentialShares(id): Promise<ShareList> { ... }
export async function revokeCredentialShare(id, shareId): Promise<void> { ... }
```

### Entities Module (~150 lines)
```typescript
// lib/api/entities.ts
export const entities = {
  list: async (params): Promise<EntityList> => { ... },
  get: async (id): Promise<Entity> => { ... },
  create: async (data): Promise<EntityId> => { ... },
  update: async (id, data): Promise<void> => { ... },
  delete: async (id): Promise<void> => { ... },
  sync: async (id): Promise<void> => { ... },
};

export const entityTypes = {
  list: async (): Promise<TypeList> => { ... },
};
```

### Services Module (~200 lines)
```typescript
// lib/api/services.ts
export async function listServices(params): Promise<ServiceList> { ... }
export async function getService(id): Promise<Service> => { ... }
export async function createService(data): Promise<ServiceId> => { ... }
export async function updateService(id, data): Promise<void> => { ... }
export async function deleteService(id): Promise<void> => { ... }
export async function deployService(id, params): Promise<Deployment> => { ... }
export async function getServiceDeployments(id): Promise<DeploymentList> => { ... }
```

### Pipelines Module (~300 lines)
```typescript
// lib/api/pipelines.ts
export async function listPipelines(params): Promise<PipelineList> { ... }
export async function getPipeline(id): Promise<Pipeline> => { ... }
export async function createPipeline(data): Promise<PipelineId> => { ... }
export async function updatePipeline(id, data): Promise<void> => { ... }
export async function deletePipeline(id): Promise<void> => { ... }
export async function runPipeline(id, params): Promise<Run> => { ... }
export async function getPipelineRuns(id): Promise<RunList> => { ... }
export async function getPipelineRun(id, runId): Promise<Run> => { ... }
export async function cancelPipelineRun(id, runId): Promise<void> => { ... }
```

### Workflows Module (~250 lines)
```typescript
// lib/api/workflows.ts
export const workflows = {
  list: async (params): Promise<WorkflowList> => { ... },
  get: async (id): Promise<Workflow> => { ... },
  create: async (data): Promise<WorkflowId> => { ... },
  update: async (id, data): Promise<void> => { ... },
  delete: async (id): Promise<void> => { ... },
  execute: async (id, params): Promise<Execution> => { ... },
  getExecutions: async (id): Promise<ExecutionList> => { ... },
};
```

### Clusters Module (~200 lines)
```typescript
// lib/api/clusters.ts
export async function listClusters(): Promise<ClusterList> { ... }
export async function getCluster(id): Promise<Cluster> => { ... }
export async function createCluster(data): Promise<ClusterId> => { ... }
export async function updateCluster(id, data): Promise<void> => { ... }
export async function deleteCluster(id): Promise<void> => { ... }
export async function testCluster(id): Promise<TestResult> => { ... }
```

### Connections Module (~250 lines)
```typescript
// lib/api/connections.ts
export async function listConnections(): Promise<ConnectionList> => { ... }
export async function getConnection(id): Promise<Connection> => { ... }
export async function createConnection(data): Promise<ConnectionId> => { ... }
export async function updateConnection(id, data): Promise<void> => { ... }
export async function deleteConnection(id): Promise<void> => { ... }
export async function testConnection(id): Promise<TestResult> => { ... }
```

### Environments Module (~150 lines)
```typescript
// lib/api/environments.ts
export async function listEnvironments(): Promise<EnvironmentList> => { ... }
export async function getEnvironment(id): Promise<Environment> => { ... }
export async function createEnvironment(data): Promise<EnvironmentId> => { ... }
export async function updateEnvironment(id, data): Promise<void> => { ... }
export async function deleteEnvironment(id): Promise<void> => { ... }
```

### Plugins Module (~200 lines)
```typescript
// lib/api/plugins.ts
export const plugins = {
  list: async (): Promise<PluginList> => { ... },
  get: async (name): Promise<Plugin> => { ... },
  toggle: async (name, enabled): Promise<void> => { ... },
  configure: async (name, config): Promise<void> => { ... },
  getStatus: async (name): Promise<Status> => { ... },
};
```

### Marketplace Module (~150 lines)
```typescript
// lib/api/marketplace.ts
export async function listMarketplacePlugins(): Promise<MarketplaceList> { ... }
export async function installPlugin(name): Promise<void> => { ... }
export async function uninstallPlugin(name): Promise<void> => { ... }
export async function getMarketplacePlugin(name): Promise<MarketplacePlugin> => { ... }
```

### Vault Module (~300 lines)
```typescript
// lib/api/vault.ts
export async function listVaultSecrets(path): Promise<SecretList> => { ... }
export async function getVaultSecret(path): Promise<Secret> => { ... }
export async function createVaultSecret(path, data): Promise<void> => { ... }
export async function updateVaultSecret(path, data): Promise<void> => { ... }
export async function deleteVaultSecret(path): Promise<void> => { ... }
export async function listVaultEngines(): Promise<EngineList> => { ... }
```

### Roles Module (~200 lines)
```typescript
// lib/api/roles.ts
export async function listRoles(): Promise<RoleList> => { ... }
export async function getRole(id): Promise<Role> => { ... }
export async function createRole(data): Promise<RoleId> => { ... }
export async function updateRole(id, data): Promise<void> => { ... }
export async function deleteRole(id): Promise<void> => { ... }
export async function assignRole(userId, roleId): Promise<void> => { ... }
export async function revokeRole(userId, roleId): Promise<void> => { ... }
```

### GitOps Module (~400 lines)
```typescript
// lib/api/gitops.ts
export async function listGitOpsRepos(): Promise<RepoList> => { ... }
export async function getGitOpsRepo(id): Promise<Repo> => { ... }
export async function createGitOpsRepo(data): Promise<RepoId> => { ... }
export async function updateGitOpsRepo(id, data): Promise<void> => { ... }
export async function deleteGitOpsRepo(id): Promise<void> => { ... }
export async function scanGitOpsRepo(id): Promise<void> => { ... }
export async function listGitOpsResources(id): Promise<ResourceList> => { ... }
export async function editGitOpsResource(id, resourceId, data): Promise<void> => { ... }
```

### Discovery Module (~250 lines)
```typescript
// lib/api/discovery.ts
export async function listDiscoverySources(): Promise<SourceList> => { ... }
export async function getDiscoverySource(id): Promise<Source> => { ... }
export async function createDiscoverySource(data): Promise<SourceId> => { ... }
export async function updateDiscoverySource(id, data): Promise<void> => { ... }
export async function deleteDiscoverySource(id): Promise<void> => { ... }
export async function runDiscovery(sourceId): Promise<void> => { ... }
```

### Audit Module (~100 lines)
```typescript
// lib/api/audit.ts
export async function listAuditLogs(params): Promise<AuditList> => { ... }
export async function getAuditLog(id): Promise<AuditLog> => { ... }
```

### Settings Module (~200 lines)
```typescript
// lib/api/settings.ts
export async function getSettings(): Promise<Settings> => { ... }
export async function updateSettings(data): Promise<void> => { ... }
export async function getSystemInfo(): Promise<SystemInfo> => { ... }
```

### Scorecards Module (~150 lines)
```typescript
// lib/api/scorecards.ts
export const scorecards = {
  list: async (): Promise<ScorecardList> => { ... },
  get: async (id): Promise<Scorecard> => { ... },
  create: async (data): Promise<ScorecardId> => { ... },
  update: async (id, data): Promise<void> => { ... },
  delete: async (id): Promise<void> => { ... },
};
```

### AI Module (~300 lines)
```typescript
// lib/api/ai.ts
export async function listAIConversations(): Promise<ConversationList> => { ... }
export async function getAIConversation(id): Promise<Conversation> => { ... }
export async function createAIConversation(): Promise<ConversationId> => { ... }
export async function sendAIMessage(id, message): Promise<Message> => { ... }
export async function listAIProviders(): Promise<ProviderList> => { ... }
export async function createAIProvider(data): Promise<ProviderId> => { ... }
```

### Virtualization Module (~350 lines)
```typescript
// lib/api/virtualization.ts
export async function listVMs(): Promise<VMList> => { ... }
export async function getVM(id): Promise<VM> => { ... }
export async function createVM(data): Promise<VMId> => { ... }
export async function startVM(id): Promise<void> => { ... }
export async function stopVM(id): Promise<void> => { ... }
export async function listContainers(): Promise<ContainerList> => { ... }
export async function getContainer(id): Promise<Container> => { ... }
```

### Health Module (~50 lines)
```typescript
// lib/api/health.ts
export const health = {
  check: async (): Promise<HealthStatus> => { ... },
  ready: async (): Promise<ReadyStatus> => { ... },
};
```

### Docker Module (~200 lines)
```typescript
// lib/api/docker.ts
export async function listDockerHosts(): Promise<HostList> => { ... }
export async function getDockerHost(id): Promise<Host> => { ... }
export async function listDockerServices(hostId): Promise<ServiceList> => { ... }
```

### S3 Module (~150 lines)
```typescript
// lib/api/s3.ts
export async function listS3Buckets(): Promise<BucketList> => { ... }
export async function listS3Objects(bucket, prefix): Promise<ObjectList> => { ... }
export async function uploadS3Object(bucket, key, file): Promise<void> => { ... }
export async function downloadS3Object(bucket, key): Promise<Blob> => { ... }
```

### Jira Module (~150 lines)
```typescript
// lib/api/jira.ts
export async function listJiraProjects(): Promise<ProjectList> => { ... }
export async function listJiraIssues(projectId): Promise<IssueList> => { ... }
export async function createJiraIssue(data): Promise<IssueId> => { ... }
```

### Helm Module (~150 lines)
```typescript
// lib/api/helm.ts
export async function listHelmRepositories(): Promise<RepoList> => { ... }
export async function getHelmRepository(id): Promise<Repo> => { ... }
export async function createHelmRepository(data): Promise<RepoId> => { ... }
export async function listHelmCharts(repoId): Promise<ChartList> => { ... }
```

## Migration Strategy

### Phase 1: Create Module Structure
1. Create `lib/api/` directory
2. Create `index.ts` with re-exports
3. Create individual module files

### Phase 2: Extract Functions
1. Copy functions from `api.ts` to appropriate modules
2. Update imports within modules
3. Ensure all dependencies are available

### Phase 3: Update Imports
1. Update all components to import from `lib/api` instead of `lib/api.ts`
2. Test all functionality

### Phase 4: Deprecate Old File
1. Mark `api.ts` as deprecated
2. Add migration guide
3. Remove after verification

## Benefits

1. **Maintainability:** Smaller files are easier to understand
2. **Testability:** Easier to write focused unit tests
3. **Code Splitting:** Better tree-shaking, smaller bundles
4. **Navigation:** Easier to find related functions
5. **Onboarding:** New developers can understand the structure faster

## Timeline

- **Day 1:** Create module structure, extract core/auth/users
- **Day 2:** Extract teams/credentials/entities
- **Day 3:** Extract services/pipelines/workflows
- **Day 4:** Extract infrastructure modules
- **Day 5:** Extract remaining modules, update imports

## Success Criteria

- [ ] All modules created
- [ ] All functions extracted
- [ ] All imports updated
- [ ] All tests passing
- [ ] Build successful
- [ ] No breaking changes

## Notes

- This is a large refactoring effort that should be done incrementally
- Each module should be extracted in a separate PR
- Comprehensive testing is critical
- Consider using TypeScript path aliases for cleaner imports
