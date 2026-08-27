-- ============================================================
-- Migration 006: Service Portal
-- ============================================================
-- Service templates, services, service deployments,
-- default scorecard seed, and RBAC role/permission seed.
-- ============================================================

-- ============================================================
-- SERVICE TEMPLATES (Self-Service Portal)
-- ============================================================

CREATE TABLE IF NOT EXISTS service_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    name            VARCHAR(128) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    category        VARCHAR(64) DEFAULT 'general',
    icon            VARCHAR(256),
    language        VARCHAR(64),
    framework       VARCHAR(64),
    dockerfile_tmpl TEXT,
    helm_chart      JSONB DEFAULT '{}',
    cicd_tmpl       TEXT,
    default_values  JSONB DEFAULT '{}',
    resource_defaults JSONB DEFAULT '{"cpu":"100m","memory":"128Mi","replicas":1}',
    tags            VARCHAR(64)[] DEFAULT '{}',
    is_enabled      BOOLEAN DEFAULT TRUE,
    is_system       BOOLEAN DEFAULT FALSE,
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_svc_tmpl_tenant ON service_templates(tenant_id);
CREATE INDEX IF NOT EXISTS idx_svc_tmpl_category ON service_templates(category);

-- Seed default templates
INSERT INTO service_templates (tenant_id, name, slug, description, category, icon, language, framework, tags, is_system, resource_defaults, default_values) VALUES
    -- Backend
    ('00000000-0000-0000-0000-000000000002', 'Node.js API', 'nodejs-api', 'Node.js Express REST API with health checks and structured logging', 'backend', '🟢', 'javascript', 'express', ARRAY['nodejs','api','rest','express'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":2}', '{"server.port":"3000","node_env":"production"}'),
    ('00000000-0000-0000-0000-000000000002', 'Go API', 'go-api', 'Go Gin REST API with graceful shutdown and Prometheus metrics', 'backend', '🔵', 'go', 'gin', ARRAY['go','api','rest','gin','prometheus'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":2}', '{"server.port":"8080","log_level":"info"}'),
    ('00000000-0000-0000-0000-000000000002', 'Python API', 'python-api', 'Python FastAPI service with async support and auto-generated docs', 'backend', '🐍', 'python', 'fastapi', ARRAY['python','api','fastapi','async'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":2}', '{"server.port":"8000","workers":"4"}'),
    ('00000000-0000-0000-0000-000000000002', 'Java Spring Boot', 'java-spring', 'Java Spring Boot REST API with Actuator health endpoints', 'backend', '☕', 'java', 'spring-boot', ARRAY['java','spring','api','rest','maven'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":2}', '{"server.port":"8080","spring.profiles.active":"prod"}'),
    ('00000000-0000-0000-0000-000000000002', '.NET API', 'dotnet-api', 'ASP.NET Core Web API with Swagger and health checks', 'backend', '🟣', 'csharp', 'aspnet', ARRAY['dotnet','csharp','api','rest'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":2}', '{"ASPNETCORE_ENVIRONMENT":"Production","ASPNETCORE_URLS":"http://+:8080"}'),
    ('00000000-0000-0000-0000-000000000002', 'Ruby on Rails', 'ruby-rails', 'Ruby on Rails API mode with PostgreSQL adapter', 'backend', '💎', 'ruby', 'rails', ARRAY['ruby','rails','api','postgresql'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":2}', '{"rails_env":"production","rails_log_level":"info"}'),
    ('00000000-0000-0000-0000-000000000002', 'PHP Laravel', 'php-laravel', 'PHP Laravel API with Octane for high performance', 'backend', '🐘', 'php', 'laravel', ARRAY['php','laravel','api','octane'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":2}', '{"app_env":"production","app_debug":"false"}'),
    ('00000000-0000-0000-0000-000000000002', 'Rust API', 'rust-api', 'Rust Axum API with Tokio runtime, low memory footprint', 'backend', '🦀', 'rust', 'axum', ARRAY['rust','api','axum','tokio'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":2}', '{"RUST_LOG":"info","server.port":"8080"}'),
    -- Frontend
    ('00000000-0000-0000-0000-000000000002', 'React SPA', 'react-spa', 'React single-page app with Vite build and Nginx serving', 'frontend', '⚛️', 'javascript', 'react', ARRAY['react','spa','vite','frontend'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":1}', '{"nginx.worker_connections":"1024"}'),
    ('00000000-0000-0000-0000-000000000002', 'Next.js App', 'nextjs-app', 'Next.js SSR/SSG application with API routes', 'frontend', '▲', 'javascript', 'nextjs', ARRAY['nextjs','react','ssr','frontend'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":2}', '{"node_env":"production","port":"3000"}'),
    ('00000000-0000-0000-0000-000000000002', 'Vue.js App', 'vue-app', 'Vue 3 SPA with Vite and Vue Router', 'frontend', '🟩', 'javascript', 'vue', ARRAY['vue','spa','vite','frontend'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":1}', '{"node_env":"production"}'),
    ('00000000-0000-0000-0000-000000000002', 'Angular App', 'angular-app', 'Angular application with Angular CLI build', 'frontend', '🅰️', 'typescript', 'angular', ARRAY['angular','spa','typescript','frontend'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{"node_env":"production"}'),
    ('00000000-0000-0000-0000-000000000002', 'Static Site', 'static-site', 'Static website (HTML/CSS/JS) served by Nginx', 'frontend', '🌐', 'html', 'none', ARRAY['static','website','frontend','nginx'], TRUE, '{"cpu":"50m","memory":"64Mi","replicas":1}', '{"nginx.worker_connections":"1024"}'),
    -- Data & Databases
    ('00000000-0000-0000-0000-000000000002', 'PostgreSQL', 'postgresql', 'PostgreSQL relational database with persistent storage', 'data', '🐘', 'any', 'postgresql', ARRAY['database','sql','postgresql','storage'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"postgres.max_connections":"100","postgres.shared_buffers":"128MB"}'),
    ('00000000-0000-0000-0000-000000000002', 'Redis', 'redis', 'Redis in-memory cache and message broker', 'data', '🔴', 'any', 'redis', ARRAY['cache','redis','in-memory','broker'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{"redis.maxmemory":"128mb","redis.maxmemory_policy":"allkeys-lru"}'),
    ('00000000-0000-0000-0000-000000000002', 'MongoDB', 'mongodb', 'MongoDB document database with replica set support', 'data', '🍃', 'any', 'mongodb', ARRAY['database','nosql','mongodb','document'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"mongodb.wiredTiger.cacheSize":"256m"}'),
    ('00000000-0000-0000-0000-000000000002', 'MySQL', 'mysql', 'MySQL relational database with InnoDB engine', 'data', '🐬', 'any', 'mysql', ARRAY['database','sql','mysql','innodb'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"mysql.max_connections":"200","mysql.innodb_buffer_pool_size":"256M"}'),
    ('00000000-0000-0000-0000-000000000002', 'Elasticsearch', 'elasticsearch', 'Elasticsearch search and analytics engine', 'data', '🔍', 'java', 'elasticsearch', ARRAY['search','elasticsearch','analytics','logging'], TRUE, '{"cpu":"1000m","memory":"1Gi","replicas":1}', '{"cluster.name":"elasticsearch","xpack.security.enabled":"false"}'),
    -- Infrastructure
    ('00000000-0000-0000-0000-000000000002', 'Nginx Proxy', 'nginx-proxy', 'Nginx reverse proxy / load balancer with custom config', 'infrastructure', '🔀', 'any', 'nginx', ARRAY['nginx','proxy','loadbalancer','reverse-proxy'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":2}', '{"nginx.worker_connections":"4096","nginx.worker_processes":"auto"}'),
    ('00000000-0000-0000-0000-000000000002', 'Traefik', 'traefik', 'Traefik edge router with auto-discovery and Let''s Encrypt', 'infrastructure', '🚦', 'go', 'traefik', ARRAY['traefik','proxy','ingress','letsencrypt'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":1}', '{"traefik.log.level":"INFO","traefik.entrypoints.web.port":"80"}'),
    ('00000000-0000-0000-0000-000000000002', 'Prometheus', 'prometheus', 'Prometheus monitoring with alerting and service discovery', 'infrastructure', '📊', 'go', 'prometheus', ARRAY['monitoring','prometheus','metrics','alerting'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"prometheus.retention":"15d","prometheus.scrape_interval":"15s"}'),
    ('00000000-0000-0000-0000-000000000002', 'Grafana', 'grafana', 'Grafana dashboards for visualization and alerting', 'infrastructure', '📈', 'go', 'grafana', ARRAY['monitoring','grafana','dashboards','visualization'], TRUE, '{"cpu":"200m","memory":"256Mi","replicas":1}', '{"grafana.server.root_url":"http://grafana.local"}'),
    -- Messaging & Streaming
    ('00000000-0000-0000-0000-000000000002', 'RabbitMQ', 'rabbitmq', 'RabbitMQ message broker with management UI', 'messaging', '🐰', 'erlang', 'rabbitmq', ARRAY['messaging','rabbitmq','amqp','queue'], TRUE, '{"cpu":"300m","memory":"384Mi","replicas":1}', '{"rabbitmq.default_vhost":"/","rabbitmq.erlang_cookie":"secret"}'),
    ('00000000-0000-0000-0000-000000000002', 'Kafka', 'kafka', 'Apache Kafka distributed event streaming platform', 'messaging', '📨', 'java', 'kafka', ARRAY['messaging','kafka','streaming','events'], TRUE, '{"cpu":"500m","memory":"1Gi","replicas":3}', '{"kafka.log.retention.hours":"168","kafka.num.partitions":"3"}'),
    ('00000000-0000-0000-0000-000000000002', 'NATS', 'nats', 'NATS lightweight cloud-native messaging system', 'messaging', '🔔', 'go', 'nats', ARRAY['messaging','nats','cloud-native','events'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{"nats.port":"4222","nats.max_payload":"1MB"}'),
    -- ML & Data Science
    ('00000000-0000-0000-0000-000000000002', 'Jupyter Notebook', 'jupyter', 'Jupyter Notebook server for data science and ML experiments', 'ml', '📓', 'python', 'jupyter', ARRAY['jupyter','notebook','ml','datascience'], TRUE, '{"cpu":"500m","memory":"1Gi","replicas":1}', '{"jupyter.token":"change-me","jupyter.port":"8888"}'),
    ('00000000-0000-0000-0000-000000000002', 'MLflow', 'mlflow', 'MLflow experiment tracking and model registry', 'ml', '🧪', 'python', 'mlflow', ARRAY['mlflow','ml','tracking','model-registry'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"mlflow.backend_store":"postgresql","mlflow.default_artifact_root":"s3://mlflow"}'),
    -- CI/CD & DevOps
    ('00000000-0000-0000-0000-000000000002', 'GitLab Runner', 'gitlab-runner', 'GitLab CI runner for executing CI/CD pipelines', 'devops', '🦊', 'go', 'gitlab', ARRAY['ci','cd','gitlab','runner','pipeline'], TRUE, '{"cpu":"500m","memory":"512Mi","replicas":1}', '{"runner.concurrent":"10","runner.executor":"kubernetes"}'),
    ('00000000-0000-0000-0000-000000000002', 'SonarQube', 'sonarqube', 'SonarQube code quality and security analysis', 'devops', '🔎', 'java', 'sonarqube', ARRAY['code-quality','sonarqube','security','analysis'], TRUE, '{"cpu":"1000m","memory":"2Gi","replicas":1}', '{"sonar.web.port":"9000","sonar.search.javaOpts":"-Xmx512m"}'),
    -- Import / Custom
    ('00000000-0000-0000-0000-000000000002', 'Helm Chart Import', 'helm-import', 'Import and deploy an existing Helm chart from any source', 'import', '📦', 'any', 'helm', ARRAY['helm','import','chart'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}'),
    ('00000000-0000-0000-0000-000000000002', 'Docker Compose Import', 'docker-compose-import', 'Import a docker-compose.yml and deploy to a Docker host', 'import', '🐳', 'any', 'docker-compose', ARRAY['docker','compose','import'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}'),
    ('00000000-0000-0000-0000-000000000002', 'Custom Container', 'custom-container', 'Deploy any Docker container image with custom settings', 'import', '📋', 'any', 'none', ARRAY['custom','container','docker','image'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}'),
    ('00000000-0000-0000-0000-000000000002', 'Blank Service', 'blank', 'Start from scratch — define everything yourself', 'import', '📝', 'any', 'none', ARRAY['blank','custom','empty'], TRUE, '{"cpu":"100m","memory":"128Mi","replicas":1}', '{}')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Add public Helm charts and Docker images to templates (latest stable releases — Aug 2026)
UPDATE service_templates SET helm_chart = '{"image":"bitnami/node:24","docs_url":"https://hub.docker.com/r/bitnami/node"}' WHERE slug = 'nodejs-api';
UPDATE service_templates SET helm_chart = '{"image":"golang:1.26-alpine","docs_url":"https://hub.docker.com/_/golang"}' WHERE slug = 'go-api';
UPDATE service_templates SET helm_chart = '{"image":"python:3.14-slim","docs_url":"https://hub.docker.com/_/python"}' WHERE slug = 'python-api';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"spring-boot","chart_version":"4.0.0","image":"bitnami/spring-boot:latest","docs_url":"https://artifacthub.io/packages/helm/bitnami/spring-boot"}' WHERE slug = 'java-spring';
UPDATE service_templates SET helm_chart = '{"image":"mcr.microsoft.com/dotnet/aspnet:10.0","docs_url":"https://hub.docker.com/_/microsoft-dotnet-aspnet"}' WHERE slug = 'dotnet-api';
UPDATE service_templates SET helm_chart = '{"image":"ruby:4.0-slim","docs_url":"https://hub.docker.com/_/ruby"}' WHERE slug = 'ruby-rails';
UPDATE service_templates SET helm_chart = '{"image":"php:8.5-fpm","docs_url":"https://hub.docker.com/_/php"}' WHERE slug = 'php-laravel';
UPDATE service_templates SET helm_chart = '{"image":"rust:1.96-slim","docs_url":"https://hub.docker.com/_/rust"}' WHERE slug = 'rust-api';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"nginx","chart_version":"19.0.0","image":"bitnami/nginx:1.30","docs_url":"https://artifacthub.io/packages/helm/bitnami/nginx"}' WHERE slug IN ('react-spa','vue-app','angular-app','static-site','nginx-proxy');
UPDATE service_templates SET helm_chart = '{"image":"node:24-alpine","docs_url":"https://hub.docker.com/_/node"}' WHERE slug = 'nextjs-app';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"postgresql","chart_version":"14.0.0","image":"bitnami/postgresql:18","docs_url":"https://artifacthub.io/packages/helm/bitnami/postgresql"}' WHERE slug = 'postgresql';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"redis","chart_version":"20.0.0","image":"bitnami/redis:8.4","docs_url":"https://artifacthub.io/packages/helm/bitnami/redis"}' WHERE slug = 'redis';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"mongodb","chart_version":"16.0.0","image":"bitnami/mongodb:8.3","docs_url":"https://artifacthub.io/packages/helm/bitnami/mongodb"}' WHERE slug = 'mongodb';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"mysql","chart_version":"12.0.0","image":"bitnami/mysql:8.4","docs_url":"https://artifacthub.io/packages/helm/bitnami/mysql"}' WHERE slug = 'mysql';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://helm.elastic.co","chart_name":"elasticsearch","chart_version":"9.5.1","image":"docker.elastic.co/elasticsearch/elasticsearch:9.5.1","docs_url":"https://artifacthub.io/packages/helm/elastic/elasticsearch"}' WHERE slug = 'elasticsearch';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://helm.traefik.io/traefik","chart_name":"traefik","chart_version":"33.1.0","image":"traefik:v3.7","docs_url":"https://artifacthub.io/packages/helm/traefik/traefik"}' WHERE slug = 'traefik';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://prometheus-community.github.io/helm-charts","chart_name":"prometheus","chart_version":"27.0.0","image":"prom/prometheus:v3.13.1","docs_url":"https://artifacthub.io/packages/helm/prometheus-community/prometheus"}' WHERE slug = 'prometheus';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://grafana.github.io/helm-charts","chart_name":"grafana","chart_version":"9.0.0","image":"grafana/grafana:13.1.3","docs_url":"https://artifacthub.io/packages/helm/grafana/grafana"}' WHERE slug = 'grafana';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"rabbitmq","chart_version":"15.0.0","image":"bitnami/rabbitmq:4.3","docs_url":"https://artifacthub.io/packages/helm/bitnami/rabbitmq"}' WHERE slug = 'rabbitmq';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.bitnami.com/bitnami","chart_name":"kafka","chart_version":"31.0.0","image":"bitnami/kafka:4.3","docs_url":"https://artifacthub.io/packages/helm/bitnami/kafka"}' WHERE slug = 'kafka';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://nats-io.github.io/k8s/helm/charts","chart_name":"nats","chart_version":"1.4.0","image":"nats:2.14","docs_url":"https://artifacthub.io/packages/helm/nats/nats"}' WHERE slug = 'nats';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://jupyterhub.github.io/helm-chart","chart_name":"jupyterhub","chart_version":"4.0.0","image":"jupyterhub/k8s-singleuser-sample:4.0.0","docs_url":"https://z2jh.jupyter.org"}' WHERE slug = 'jupyter';
UPDATE service_templates SET helm_chart = '{"image":"ghcr.io/mlflow/mlflow:v3.15.1","docs_url":"https://mlflow.org/docs/latest/index.html"}' WHERE slug = 'mlflow';
UPDATE service_templates SET helm_chart = '{"repo_url":"https://charts.gitlab.io","chart_name":"gitlab-runner","chart_version":"0.91.0","image":"gitlab/gitlab-runner:alpine","docs_url":"https://artifacthub.io/packages/helm/gitlab/gitlab-runner"}' WHERE slug = 'gitlab-runner';
UPDATE service_templates SET helm_chart = '{"image":"sonarqube:2026-lts-community","docs_url":"https://hub.docker.com/_/sonarqube"}' WHERE slug = 'sonarqube';

-- ============================================================
-- SERVICES (created from templates)
-- ============================================================

CREATE TABLE IF NOT EXISTS services (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    template_id     UUID REFERENCES service_templates(id),
    name            VARCHAR(128) NOT NULL,
    slug            VARCHAR(128) NOT NULL,
    description     TEXT DEFAULT '',
    owner_team_id   UUID REFERENCES teams(id),
    language        VARCHAR(64),
    framework       VARCHAR(64),
    gitlab_project_url VARCHAR(512),
    helm_chart_url  VARCHAR(512),
    image_repository VARCHAR(256),
    namespace       VARCHAR(63) DEFAULT 'default',
    status          VARCHAR(32) DEFAULT 'creating',
    resource_config JSONB DEFAULT '{}',
    environment_variables JSONB DEFAULT '{}',
    vault_secrets   JSONB DEFAULT '{}',
    deployment_strategy VARCHAR(32) DEFAULT 'rolling',
    target_clusters UUID[] DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_services_tenant ON services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_services_status ON services(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_services_template ON services(template_id);

-- ============================================================
-- SERVICE DEPLOYMENT STAGES (tracking dev→testing→staging→prod)
-- ============================================================

CREATE TABLE IF NOT EXISTS service_deployments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL,
    service_id      UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    environment     VARCHAR(32) NOT NULL,
    cluster_id      UUID REFERENCES clusters(id),
    namespace       VARCHAR(63),
    branch          VARCHAR(128),
    image_tag       VARCHAR(128),
    helm_release    VARCHAR(128),
    deploy_type     VARCHAR(32) DEFAULT 'automatic',
    status          VARCHAR(32) DEFAULT 'pending',
    verification_status VARCHAR(32) DEFAULT 'pending',
    verification_details JSONB DEFAULT '{}',
    flux_synced     BOOLEAN DEFAULT FALSE,
    pods_ready      INTEGER DEFAULT 0,
    pods_total      INTEGER DEFAULT 0,
    mr_url          VARCHAR(512),
    pipeline_url    VARCHAR(512),
    deployed_at     TIMESTAMPTZ,
    verified_at     TIMESTAMPTZ,
    promoted_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_svc_deploy_tenant ON service_deployments(tenant_id);
CREATE INDEX IF NOT EXISTS idx_svc_deploy_service ON service_deployments(service_id);
CREATE INDEX IF NOT EXISTS idx_svc_deploy_env ON service_deployments(environment);

-- ============================================================
-- SEED: DEFAULT SCORECARD — Production Readiness
-- ============================================================

INSERT INTO scorecards (id, tenant_id, name, description, enabled, config)
VALUES (
    '10000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000002',
    'Production Readiness',
    'Evaluates whether a service meets production readiness criteria based on CNCF best practices.',
    TRUE,
    '{"levels": ["bronze", "silver", "gold", "platinum"], "thresholds": {"bronze": 25, "silver": 50, "gold": 75, "platinum": 90}}'
) ON CONFLICT (name, tenant_id) DO NOTHING;

INSERT INTO scorecard_rules (scorecard_id, name, description, expression, weight, pass_message, fail_message, severity) VALUES
    ('10000000-0000-0000-0000-000000000001', 'Has Health Endpoint', 'Service exposes a /healthz or /health endpoint', 'metadata.health_endpoint == true', 8, 'Health endpoint configured', 'Missing health endpoint — required for liveness probes', 'error'),
    ('10000000-0000-0000-0000-000000000001', 'Has Readiness Endpoint', 'Service exposes a /readyz or /ready endpoint', 'metadata.readiness_endpoint == true', 8, 'Readiness endpoint configured', 'Missing readiness endpoint — required for traffic routing', 'error'),
    ('10000000-0000-0000-0000-000000000001', 'Has Owner Team', 'Service has an owning team assigned', 'owner_team_id != null', 5, 'Owner team assigned', 'No owner team — every service needs a responsible team', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Has Description', 'Service has a meaningful description', 'description != ""', 3, 'Description provided', 'Missing service description', 'info'),
    ('10000000-0000-0000-0000-000000000001', 'Resource Limits Set', 'CPU and memory limits are defined', 'resource_config.cpu != null && resource_config.memory != null', 7, 'Resource limits configured', 'Missing resource limits — can cause noisy-neighbor issues', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Replicas >= 2', 'Service runs at least 2 replicas for HA', 'resource_config.replicas >= 2', 6, 'Multiple replicas configured', 'Single replica — not resilient to failures', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Has GitLab Project', 'Service is linked to a GitLab project', 'gitlab_project_url != null && gitlab_project_url != ""', 4, 'GitLab project linked', 'No GitLab project URL set', 'info'),
    ('10000000-0000-0000-0000-000000000001', 'Has Helm Chart', 'Service has an associated Helm chart', 'helm_chart_url != null && helm_chart_url != ""', 5, 'Helm chart configured', 'No Helm chart — GitOps deployment requires a chart', 'warning'),
    ('10000000-0000-0000-0000-000000000001', 'Deployment Strategy Set', 'A deployment strategy is defined', 'deployment_strategy != null && deployment_strategy != ""', 4, 'Deployment strategy defined', 'No deployment strategy — consider rolling or canary', 'info'),
    ('10000000-0000-0000-0000-000000000001', 'Environment Variables Documented', 'Service has environment variables configured', 'environment_variables != null', 2, 'Environment variables present', 'No environment variables defined', 'info')
ON CONFLICT DO NOTHING;

-- ============================================================
-- SEED: DEFAULT RBAC ROLES
-- ============================================================

INSERT INTO roles (id, tenant_id, name, slug, description, is_system, scope) VALUES
    ('20000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000002', 'Platform Admin', 'admin', 'Full access to all platform resources', TRUE, 'tenant'),
    ('20000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000002', 'Developer', 'developer', 'Can manage services, workflows, and deployments', TRUE, 'tenant'),
    ('20000000-0000-0000-0000-000000000003', '00000000-0000-0000-0000-000000000002', 'Viewer', 'viewer', 'Read-only access to all resources', TRUE, 'tenant')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Admin permissions: full access
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000001', r, a, 'allow'
FROM unnest(ARRAY['entities','services','deployments','workflows','clusters','connections','scorecards','plugins','roles','audit','settings','policies','vault']) AS r
CROSS JOIN unnest(ARRAY['create','read','update','delete']) AS a
ON CONFLICT DO NOTHING;

-- Developer permissions
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000002', r, a, 'allow'
FROM unnest(ARRAY['entities','services','deployments','workflows','clusters','connections','scorecards']) AS r
CROSS JOIN unnest(ARRAY['create','read','update']) AS a
ON CONFLICT DO NOTHING;

INSERT INTO permissions (role_id, resource, action, effect) VALUES
    ('20000000-0000-0000-0000-000000000002', 'audit', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'policies', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'read', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'create', 'allow'),
    ('20000000-0000-0000-0000-000000000002', 'vault', 'delete', 'allow')
ON CONFLICT DO NOTHING;

-- Viewer permissions: read-only
INSERT INTO permissions (role_id, resource, action, effect)
SELECT '20000000-0000-0000-0000-000000000003', r, 'read', 'allow'
FROM unnest(ARRAY['entities','services','deployments','workflows','clusters','connections','scorecards','plugins','audit','policies']) AS r
ON CONFLICT DO NOTHING;

-- Record migration version
INSERT INTO schema_migrations (version, description) VALUES
    (6, 'Add service_templates, services, service_deployments + seed scorecard and RBAC roles')
ON CONFLICT DO NOTHING;
