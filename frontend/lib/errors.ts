// Maps common backend error messages to actionable hints for beginners.
// Original error stays visible (muted), hint is shown prominently.

interface ErrorHint {
  match: string[];
  hint: string;
}

const HINTS: ErrorHint[] = [
  {
    match: ['argocd not configured', 'argocd: not', 'argo cd not'],
    hint: 'ArgoCD is not installed on the target cluster. Connect a cluster that runs ArgoCD, or use a FluxCD-based workflow instead.',
  },
  {
    match: ['flux not installed', 'fluxcd not', 'no gitops engine'],
    hint: 'No GitOps engine (FluxCD/ArgoCD) was detected on the cluster. Install one, or deploy directly via the Deployments page.',
  },
  {
    match: ['connection refused', 'no such host', 'dial tcp', 'network is unreachable', 'i/o timeout'],
    hint: 'The platform cannot reach this address. Check the URL, and make sure the host is reachable from the PEPA server (VPN / firewall).',
  },
  {
    match: ['token was revoked', 're-authorize', 'reauthorize', 'token revoked'],
    hint: 'The API token was revoked or expired. Go to Connections and update the token to re-authorize.',
  },
  {
    match: ['401', 'unauthorized', 'invalid token', 'token is invalid', 'authentication failed'],
    hint: 'Authentication failed. Verify the API token is correct, not expired, and has the required scopes.',
  },
  {
    match: ['403', 'forbidden', 'permission denied', 'access denied'],
    hint: 'Access denied. The token or user does not have permission for this operation.',
  },
  {
    match: ['404', 'not found'],
    hint: 'The requested resource was not found. It may have been deleted or the name/ID is misspelled.',
  },
  {
    match: ['kubeconfig', 'no configuration has been provided', 'invalid configuration'],
    hint: 'The kubeconfig looks invalid. Paste the full YAML (from ~/.kube/config) and make sure it contains server URL and credentials.',
  },
  {
    match: ['duplicate key', 'already exists', 'unique constraint'],
    hint: 'An item with the same identifier already exists. Use a different name or update the existing item instead.',
  },
  {
    match: ['plugin not found', 'no plugin', 'provider not found', 'not registered'],
    hint: 'The required plugin is not loaded or not enabled. Check the Plugins page and make sure the plugin is enabled.',
  },
  {
    match: ['context deadline exceeded', 'timeout', 'timed out'],
    hint: 'The operation timed out. The target system may be slow or unreachable — try again or check its health.',
  },
];

export interface FriendlyError {
  message: string;
  hint?: string;
}

/** Returns the original message plus an actionable hint when one is known. */
export function friendlyError(err: unknown): FriendlyError {
  const message = err instanceof Error ? err.message : String(err || 'Unknown error');
  const lower = message.toLowerCase();
  const found = HINTS.find(h => h.match.some(m => lower.includes(m.toLowerCase())));
  return { message, hint: found?.hint };
}
