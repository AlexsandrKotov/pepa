// Plain-English glossary of PEPA concepts, used by the ConceptHelp component.

export interface GlossaryEntry {
  what: string;
  why: string;
  example: string;
}

export const glossary: Record<string, GlossaryEntry> = {
  connection: {
    what: 'A saved link from PEPA to an external system: a Kubernetes cluster, GitLab, Jira, and so on. PEPA stores the credentials and checks that the link works.',
    why: 'Everything PEPA does — browsing repos, deploying, tracking issues — happens through connections. You set them up once and reuse them everywhere.',
    example: 'A "Production Cluster" connection holds a kubeconfig; a "GitLab" connection holds your GitLab URL and API token.',
  },
  cluster: {
    what: 'A Kubernetes cluster that PEPA can talk to, created automatically when you add a Kubernetes connection with a kubeconfig.',
    why: 'Clusters are where your services actually run. PEPA shows their health, namespaces and workloads, and deploys to them.',
    example: 'dev-cluster, staging-cluster, prod-cluster — one connection per cluster.',
  },
  service: {
    what: 'A software product you build and maintain: a web app, an API, a background worker, a library. Services are the main units in the catalog.',
    why: 'The catalog answers "what do we run, who owns it, and where does it live". Every deployment, workflow and scorecard attaches to a service.',
    example: '"payments-api" — a Go service owned by team-backend, deployed to prod-cluster.',
  },
  entity: {
    what: 'The generic building block behind the catalog. Services, teams, environments, APIs and domains are all entities with a type, metadata and relationships.',
    why: 'One flexible model lets you describe your whole platform — who owns what, what depends on what — and visualize it as a graph.',
    example: 'Entity "team-backend" (type: team) → "owns" → entity "payments-api" (type: service).',
  },
  workflow: {
    what: 'An automation made of steps that run in order (or in parallel): build, test, deploy, notify. Steps can use plugins, conditions and approvals.',
    why: 'Instead of clicking through tools manually, you define the process once and run it with one button — the same way every time.',
    example: 'A CI/CD workflow: run tests → build image → deploy via GitOps → send Slack notification.',
  },
  template: {
    what: 'A ready-made workflow or service blueprint you can instantiate instead of writing everything from scratch.',
    why: 'Templates encode best practices. Beginners start from a template and customize later.',
    example: 'The "CI/CD Pipeline" template creates a full build-test-deploy workflow in one click.',
  },
  deployment: {
    what: 'One act of delivering a service version to a cluster: which service, which environment, which status (pending, deploying, deployed, failed).',
    why: 'Deployments give you a single feed of what went where and when, with rollback context if something breaks.',
    example: 'Deploy payments-api v1.4.2 to staging → status becomes "deployed" after the rollout succeeds.',
  },
  gitops: {
    what: 'A deployment approach where the desired state lives in Git, and an operator in the cluster (FluxCD or ArgoCD) continuously syncs it. PEPA detects which engine a cluster uses.',
    why: 'No manual kubectl: changes are auditable in Git, and rollbacks are just git reverts.',
    example: 'Merge to the main branch → FluxCD notices the change → the cluster updates itself.',
  },
  plugin: {
    what: 'A small standalone program that adds integration capabilities to PEPA (GitLab, Jira, Slack, ArgoCD, FluxCD...) via a standard interface.',
    why: 'Plugins make PEPA extensible: new tools can be added without changing the core, and you can write your own in Go.',
    example: 'The gitlab plugin exposes actions like list_projects and create_merge_request that workflows can call.',
  },
  environment: {
    what: 'A logical stage where services run: dev, staging, production. Environments carry their own variables and settings.',
    why: 'Promoting a service through environments (dev → staging → prod) is the core of a safe delivery pipeline.',
    example: 'payments-api is deployed to dev on every merge, to staging after review, to prod after approval.',
  },
};

export type GlossaryTerm = keyof typeof glossary;
