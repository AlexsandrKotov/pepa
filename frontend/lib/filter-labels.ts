/**
 * Single source of truth for filter terminology across the whole frontend.
 * Field labels are sentence case ("Environment" / "All environments"),
 * raw values are mapped to human labels ("connected" -> "Connected").
 *
 * Pages must never hardcode option captions — import fieldLabel / valueLabel instead.
 */

export type Tone = 'success' | 'danger' | 'warning' | 'info' | 'accent' | 'muted';

export const FIELD_LABELS: Record<string, string> = {
  search: 'Search',
  status: 'Status',
  environment: 'Environment',
  health: 'Health',
  gitops: 'GitOps',
  engine: 'Engine',
  cluster: 'Cluster',
  namespace: 'Namespace',
  source: 'Source',
  type: 'Type',
  severity: 'Severity',
  team: 'Team',
  role: 'Role',
  provider: 'Provider',
  result: 'Result',
  sort: 'Sort',
};

export const VALUE_LABELS: Record<string, string> = {
  // Connection / cluster status
  connected: 'Connected',
  disconnected: 'Disconnected',
  syncing: 'Syncing',
  pending: 'Pending',
  healthy: 'Healthy',
  degraded: 'Degraded',
  progressing: 'Progressing',
  suspended: 'Suspended',
  unknown: 'Unknown',
  running: 'Running',
  stopped: 'Stopped',
  deploying: 'Deploying',
  failed: 'Failed',
  error: 'Error',
  success: 'Success',
  active: 'Active',
  inactive: 'Inactive',
  configured: 'Configured',
  // Environments
  production: 'Production',
  prod: 'Production',
  staging: 'Staging',
  stage: 'Staging',
  uat: 'UAT',
  dev: 'Dev',
  development: 'Development',
  test: 'Test',
  local: 'Local',
  // GitOps engines
  flux: 'FluxCD',
  fluxcd: 'FluxCD',
  argo: 'ArgoCD',
  argocd: 'ArgoCD',
  none: 'None',
  all: 'All',
  // Severities
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
};

const TONE_BY_VALUE: Record<string, Tone> = {
  connected: 'success',
  healthy: 'success',
  active: 'success',
  running: 'success',
  deployed: 'success',
  success: 'success',
  disconnected: 'danger',
  failed: 'danger',
  error: 'danger',
  critical: 'danger',
  degraded: 'warning',
  syncing: 'warning',
  warning: 'warning',
  high: 'warning',
  pending: 'info',
  progressing: 'info',
  deploying: 'info',
  medium: 'info',
  stopped: 'muted',
  suspended: 'muted',
  unknown: 'muted',
  none: 'muted',
  low: 'muted',
  production: 'danger',
  prod: 'danger',
  staging: 'warning',
  stage: 'warning',
  uat: 'warning',
  dev: 'success',
  development: 'success',
  test: 'info',
  local: 'muted',
};

export function fieldLabel(field: string): string {
  return FIELD_LABELS[field] ?? field.charAt(0).toUpperCase() + field.slice(1);
}

/** Human label for a raw filter value, e.g. "auto_created" -> "Auto created". */
export function valueLabel(value: string): string {
  const key = value.toLowerCase();
  if (VALUE_LABELS[key]) return VALUE_LABELS[key];
  return key.replace(/[_-]+/g, ' ').replace(/\b\w/g, c => c.toUpperCase());
}

export function valueTone(value: string): Tone {
  return TONE_BY_VALUE[value.toLowerCase()] ?? 'muted';
}

/** "All environments" — sentence case, used as the neutral option caption. */
export function allLabel(field: string): string {
  return `All ${fieldLabel(field).toLowerCase()}s`;
}
