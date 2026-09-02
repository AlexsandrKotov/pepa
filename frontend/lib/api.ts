// Determine the API base URL at runtime:
// - If NEXT_PUBLIC_API_URL is explicitly set to a non-empty value, use it.
// - If accessed through nginx (port 80/443 or no port), use relative URLs
//   so requests go through the same origin (no CORS).
// - If accessed directly (e.g. port 3000 during dev or Docker direct access),
//   use the API server on port 8088.
const _raw = process.env.NEXT_PUBLIC_API_URL;

export function getBase(): string {
  // Explicit non-empty env var takes priority
  if (_raw !== undefined && _raw !== '') return _raw;
  // In browser: detect access method
  if (typeof window !== 'undefined') {
    const port = window.location.port;
    // Port 80/443 (or empty) means nginx is proxying — use relative URL
    if (port === '' || port === '80' || port === '443') {
      return '';
    }
    // Direct access (e.g. :3000) — point to the API server
    return `${window.location.protocol}//${window.location.hostname}:8088`;
  }
  // Server-side fallback
  return 'http://localhost:8088';
}

// ── Auth session management ─────────────────────────────────
// The JWT is stored by the server in an httpOnly cookie (pepa_token),
// which is not readable from JavaScript — XSS cannot steal the session.
// Only the non-sensitive user profile is kept in localStorage.
//
// Bootstrap exception: during the first-run bootstrap flow, the frontend
// and API are on different origins (cross-origin cookies are unreliable).
// We temporarily hold the bootstrap JWT in a module-level variable and
// pass it via Authorization header. It is cleared immediately after the
// password change completes and never persisted anywhere.
const TOKEN_KEY = 'pepa_token'; // legacy — migrated away, cleaned up below
const USER_KEY = 'pepa_user';
let _bootstrapJwt: string | null = null;

export function getToken(): string | null {
  // Tokens are no longer accessible to client-side code (httpOnly cookie).
  return null;
}

export function setToken(_token: string): void {
  // No-op: the server sets the httpOnly cookie on login/refresh.
  // Kept for backward compatibility with older call sites.
}

export function removeToken(): void {
  _bootstrapJwt = null;
  if (typeof window === 'undefined') return;
  localStorage.removeItem(TOKEN_KEY); // clean up legacy stored token
  localStorage.removeItem(USER_KEY);
}

export function getStoredUser(): { id: string; email: string; name: string; roles: string[]; permissions: string[] } | null {
  if (typeof window === 'undefined') return null;
  const raw = localStorage.getItem(USER_KEY);
  if (!raw) return null;
  try { return JSON.parse(raw); } catch { return null; }
}

export function setStoredUser(user: { id: string; email: string; name: string; roles: string[]; permissions?: string[] } | null): void {
  if (typeof window === 'undefined') return;
  if (user === null) {
    localStorage.removeItem(USER_KEY);
    return;
  }
  localStorage.setItem(USER_KEY, JSON.stringify({ ...user, permissions: user.permissions || [] }));
}

export function isAuthenticated(): boolean {
  // The session cookie is httpOnly and cannot be inspected; presence of the
  // stored user profile indicates a logged-in session (401s re-validate it).
  return !!getStoredUser();
}

export function authHeaders(): Record<string, string> {
  // During bootstrap, pass the JWT via header because cross-origin cookies
  // are unreliable. After bootstrap, auth travels via the httpOnly cookie.
  if (_bootstrapJwt) return { Authorization: `Bearer ${_bootstrapJwt}` };
  return {};
}

// ── Session verification on 401 ──────────────────────────────
// When any API call returns 401, we don't immediately clear auth.
// Instead, we verify the session by calling /auth/me. If that also
// returns 401, the session is truly dead and we redirect to /login.
// This prevents spurious logouts from transient 401s (cookie timing,
// cross-origin issues right after login, etc.).
let _sessionCheck: Promise<boolean> | null = null;

function verifySession(): Promise<boolean> {
  if (_sessionCheck) return _sessionCheck;
  _sessionCheck = fetch(`${getBase()}/api/v1/auth/me`, {
    headers: authHeaders(),
    cache: 'no-store',
    credentials: 'include',
  })
    .then(res => {
      _sessionCheck = null;
      return res.ok; // true = session alive, false = session dead
    })
    .catch(() => {
      _sessionCheck = null;
      return true; // network error ≠ session expired; don't clear auth
    });
  return _sessionCheck;
}

// ── Simple in-memory cache with TTL and request deduplication ──
const cache = new Map<string, { data: unknown; expiry: number }>();
const inflight = new Map<string, Promise<unknown>>();
const CACHE_TTL = 15_000; // 15 seconds for GET requests
const SLOW_ENDPOINTS = new Set(['/api/v1/clusters', '/api/v1/discovery/services']);
const SLOW_TTL = 30_000; // 30 seconds for slow endpoints

// Endpoint-specific TTL overrides (client-side)
const ENDPOINT_TTL: Record<string, number> = {
  '/api/v1/plugins': 300_000,         // 5 min — rarely change
  '/api/v1/platform-settings': 300_000, // 5 min — rarely change
};

// Server-side cache (Node.js SSR) — longer TTL since data changes less frequently
const serverCache = new Map<string, { data: unknown; expiry: number }>();
const serverInflight = new Map<string, Promise<unknown>>();
const SERVER_CACHE_TTL = 30_000; // 30 seconds default for SSR
const SERVER_SLOW_TTL = 120_000; // 2 minutes for slow endpoints (discovery, clusters)

function getServerTtl(path: string): number {
  for (const [prefix, ttl] of Object.entries(ENDPOINT_TTL)) {
    if (path.startsWith(prefix)) return ttl;
  }
  for (const slow of SLOW_ENDPOINTS) {
    if (path.startsWith(slow)) return SERVER_SLOW_TTL;
  }
  return SERVER_CACHE_TTL;
}

function cacheKey(path: string): string {
  return path;
}

function getTtl(path: string): number {
  for (const [prefix, ttl] of Object.entries(ENDPOINT_TTL)) {
    if (path.startsWith(prefix)) return ttl;
  }
  for (const slow of SLOW_ENDPOINTS) {
    if (path.startsWith(slow)) return SLOW_TTL;
  }
  return CACHE_TTL;
}

function invalidateCache(path?: string) {
  if (path) {
    const basePath = path.split('?')[0];
    // Invalidate matching prefix AND parent collection paths
    // e.g. DELETE /api/v1/teams/123 should also invalidate /api/v1/teams
    const segments = basePath.split('/');
    const pathsToInvalidate = [basePath];
    // Walk up the path tree to invalidate parent collections
    for (let i = segments.length - 1; i >= 2; i--) {
      pathsToInvalidate.push(segments.slice(0, i).join('/'));
    }
    for (const key of cache.keys()) {
      if (pathsToInvalidate.some(p => key.startsWith(p))) cache.delete(key);
    }
  } else {
    cache.clear();
  }
}

async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const method = (options?.method || 'GET').toUpperCase();
  const headers = { 'Content-Type': 'application/json', ...authHeaders(), ...options?.headers };

  // Invalidate cache on mutations
  if (method !== 'GET') {
    invalidateCache(path);
    const res = await fetch(`${getBase()}${path}`, {
      ...options,
      headers,
      cache: 'no-store',
      credentials: 'include',
    });
    if (res.status === 401 && typeof window !== 'undefined') {
      const alive = await verifySession();
      if (!alive) {
        removeToken();
        if (window.location.pathname !== '/login') {
          // eslint-disable-next-line @next/next/no-location-assign-relative-destination
          window.location.href = '/login';
        }
        throw new Error('Session expired');
      }
      // Session is alive — this 401 was transient. Retry once.
      const retry = await fetch(`${getBase()}${path}`, { ...options, headers, cache: 'no-store', credentials: 'include' });
      if (!retry.ok) {
        const body = await retry.json().catch(() => ({}));
        throw new Error(body.error || `API error: ${retry.status}`);
      }
      return retry.json();
    }
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `API error: ${res.status}`);
    }
    return res.json();
  }

  // Client-side: use cache + dedup
  if (typeof window !== 'undefined') {
    const key = cacheKey(path);
    const cached = cache.get(key);
    if (cached && cached.expiry > Date.now()) {
      return cached.data as T;
    }

    // Deduplicate concurrent requests
    const pending = inflight.get(key);
    if (pending) return pending as Promise<T>;

    const promise = (async () => {
      try {
        const res = await fetch(`${getBase()}${path}`, {
          ...options,
          headers,
          cache: 'no-store',
          credentials: 'include',
        });
        if (res.status === 401 && typeof window !== 'undefined') {
          const alive = await verifySession();
          if (!alive) {
            removeToken();
            if (window.location.pathname !== '/login') {
              // eslint-disable-next-line @next/next/no-location-assign-relative-destination
              window.location.href = '/login';
            }
            throw new Error('Session expired');
          }
          // Session alive — transient 401. Retry once.
          const retry = await fetch(`${getBase()}${path}`, { ...options, headers, cache: 'no-store', credentials: 'include' });
          if (!retry.ok) {
            const body = await retry.json().catch(() => ({}));
            throw new Error(body.error || `API error: ${retry.status}`);
          }
          const data = await retry.json();
          cache.set(key, { data, expiry: Date.now() + getTtl(path) });
          return data as T;
        }
        if (!res.ok) {
          const body = await res.json().catch(() => ({}));
          throw new Error(body.error || `API error: ${res.status}`);
        }
        const data = await res.json();
        cache.set(key, { data, expiry: Date.now() + getTtl(path) });
        return data as T;
      } finally {
        inflight.delete(key);
      }
    })();

    inflight.set(key, promise);
    return promise as Promise<T>;
  }

  // Server-side: use cache + dedup for SSR
  const key = cacheKey(path);
  const sCached = serverCache.get(key);
  if (sCached && sCached.expiry > Date.now()) {
    return sCached.data as T;
  }

  const sPending = serverInflight.get(key);
  if (sPending) return sPending as Promise<T>;

  const sPromise = (async () => {
    try {
      const res = await fetch(`${getBase()}${path}`, {
        ...options,
        headers,
        cache: 'no-store',
        credentials: 'include',
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || `API error: ${res.status}`);
      }
      const data = await res.json();
      serverCache.set(key, { data, expiry: Date.now() + getServerTtl(path) });
      return data as T;
    } finally {
      serverInflight.delete(key);
    }
  })();

  serverInflight.set(key, sPromise);
  return sPromise as Promise<T>;
}

// ── Auth API ─────────────────────────────────────────────────

export async function login(email: string, password: string): Promise<{ token: string; user: { id: string; email: string; name: string; roles: string[] }; must_change_password: boolean }> {
  const res = await fetch(`${getBase()}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
    credentials: 'include',
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'Login failed');
  }
  const data = await res.json();
  // When must_change_password is set, store the JWT for the subsequent
  // password-change request (cross-origin cookies are unreliable).
  if (data.must_change_password && data.token) {
    _bootstrapJwt = data.token;
  }
  // Session cookie is set by the server; only store the user profile.
  setStoredUser(data.user);
  return data;
}

export async function logout(): Promise<void> {
  _bootstrapJwt = null;
  try {
    await fetch(`${getBase()}/api/v1/auth/logout`, {
      method: 'POST',
      credentials: 'include',
    });
  } catch {
    // Best effort — clear local state regardless.
  }
  removeToken();
}

// ── OIDC/SSO API ─────────────────────────────────────────────

export async function getOIDCConfig(): Promise<{ enabled: boolean; issuer?: string; client_id?: string; redirect_url?: string; scopes?: string[] }> {
  const res = await fetch(`${getBase()}/api/v1/auth/oidc/config`, {
    credentials: 'include',
  });
  if (!res.ok) {
    return { enabled: false };
  }
  return res.json();
}

export async function getOIDCLoginURL(): Promise<{ redirect_url: string }> {
  const res = await fetch(`${getBase()}/api/v1/auth/oidc/login`, {
    method: 'GET',
    credentials: 'include',
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'Failed to get OIDC login URL');
  }
  return res.json();
}

// Admin: get full OID config (client_secret is masked)
export async function getOIDCAdminConfig(): Promise<{
  enabled: boolean;
  issuer: string;
  client_id: string;
  client_secret: string;
  redirect_url: string;
  scopes: string[];
}> {
  return fetchAPI('/api/v1/settings/oidc/config');
}

// ── Azure AD API ─────────────────────────────────────────────

export async function getAzureConfig(): Promise<{ enabled: boolean }> {
  const res = await fetch(`${getBase()}/api/v1/auth/azure/config`, {
    credentials: 'include',
  });
  if (!res.ok) {
    return { enabled: false };
  }
  return res.json();
}

export async function getAzureLoginURL(): Promise<{ redirect_url: string }> {
  const res = await fetch(`${getBase()}/api/v1/auth/azure/login`, {
    method: 'GET',
    credentials: 'include',
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'Failed to get Azure AD login URL');
  }
  return res.json();
}

export async function getAzureAdminConfig(): Promise<{
  enabled: boolean;
  tenant_id: string;
  client_id: string;
  client_secret: string;
  redirect_url: string;
}> {
  return fetchAPI('/api/v1/settings/azure/config');
}

// ── Google OAuth API ──────────────────────────────────────────

export async function getGoogleConfig(): Promise<{ enabled: boolean }> {
  const res = await fetch(`${getBase()}/api/v1/auth/google/config`, {
    credentials: 'include',
  });
  if (!res.ok) {
    return { enabled: false };
  }
  return res.json();
}

export async function getGoogleLoginURL(): Promise<{ redirect_url: string }> {
  const res = await fetch(`${getBase()}/api/v1/auth/google/login`, {
    method: 'GET',
    credentials: 'include',
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'Failed to get Google login URL');
  }
  return res.json();
}

export async function getGoogleAdminConfig(): Promise<{
  enabled: boolean;
  client_id: string;
  client_secret: string;
  redirect_url: string;
}> {
  return fetchAPI('/api/v1/settings/google/config');
}

// ── GitHub OAuth API ──────────────────────────────────────────

export async function getGitHubConfig(): Promise<{ enabled: boolean }> {
  const res = await fetch(`${getBase()}/api/v1/auth/github/config`, {
    credentials: 'include',
  });
  if (!res.ok) {
    return { enabled: false };
  }
  return res.json();
}

export async function getGitHubLoginURL(): Promise<{ redirect_url: string }> {
  const res = await fetch(`${getBase()}/api/v1/auth/github/login`, {
    method: 'GET',
    credentials: 'include',
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'Failed to get GitHub login URL');
  }
  return res.json();
}

export async function getGitHubAdminConfig(): Promise<{
  enabled: boolean;
  client_id: string;
  client_secret: string;
  redirect_url: string;
}> {
  return fetchAPI('/api/v1/settings/github/config');
}

// ── LDAP API ─────────────────────────────────────────────────

export async function getLDAPConfig(): Promise<{ enabled: boolean }> {
  const res = await fetch(`${getBase()}/api/v1/auth/ldap/config`, {
    credentials: 'include',
  });
  if (!res.ok) {
    return { enabled: false };
  }
  return res.json();
}

export async function ldapLogin(email: string, password: string): Promise<{
  token: string;
  expires_in: number;
  user: { id: string; email: string; name: string; roles: string[] };
}> {
  const res = await fetch(`${getBase()}/api/v1/auth/ldap/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ email, password }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'LDAP login failed');
  }
  return res.json();
}

export async function getLDAPAdminConfig(): Promise<{
  enabled: boolean;
  url: string;
  bind_dn: string;
  bind_password: string;
  base_dn: string;
  user_filter: string;
  group_filter: string;
  email_attr: string;
  name_attr: string;
  start_tls: boolean;
  insecure_skip_verify: boolean;
  ca_certificate: string;
  group_mapping: Record<string, string>;
}> {
  return fetchAPI('/api/v1/settings/ldap/config');
}

export async function testLDAPConnection(config: {
  url: string;
  bind_dn: string;
  bind_password: string;
  base_dn: string;
  start_tls: boolean;
  insecure_skip_verify: boolean;
  ca_certificate: string;
}): Promise<{ status: string; message: string }> {
  return fetchAPI('/api/v1/settings/ldap/test', {
    method: 'POST',
    body: JSON.stringify(config),
  });
}

export async function getMe(): Promise<{ user: { id: string; email: string; name: string; is_active: boolean }; roles: string[]; permissions: string[] }> {
  // Use raw fetch instead of fetchAPI to avoid the global 401→redirect behavior.
  // getMe() is called on page load to check session validity — a 401 here
  // means "not authenticated" and should be handled gracefully by the caller.
  const res = await fetch(`${getBase()}/api/v1/auth/me`, {
    headers: authHeaders(),
    cache: 'no-store',
    credentials: 'include',
  });
  if (!res.ok) {
    throw new Error(res.status === 401 ? 'Not authenticated' : `API error: ${res.status}`);
  }
  return res.json();
}

/**
 * Refresh the JWT session by calling /api/v1/auth/refresh.
 * The server sets a new httpOnly cookie with an extended expiry.
 * Returns the new token (for bootstrap header fallback) and expiry.
 */
export async function refreshMe(): Promise<{ token: string; expires_in: number } | null> {
  try {
    const res = await fetch(`${getBase()}/api/v1/auth/refresh`, {
      method: 'POST',
      headers: authHeaders(),
      credentials: 'include',
    });
    if (!res.ok) return null;
    return res.json();
  } catch {
    return null;
  }
}

export async function resetMyPassword(currentPassword: string, newPassword: string): Promise<{ user: { id: string; email: string; name: string }; roles: string[] } | null> {
  const data = await fetchAPI<{ message: string; user: { id: string; email: string; name: string }; roles: string[] }>('/api/v1/auth/me/reset-password', {
    method: 'POST',
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
  // Keep _bootstrapJwt for subsequent cross-origin requests.
  // The cookie set by the server may not work cross-origin, so the
  // Authorization header fallback is needed until the page is refreshed.
  // Refresh stored user with fresh data from server
  if (data?.user) {
    setStoredUser({ ...data.user, roles: data.roles || [] });
  }
  return data;
}

export async function getBootstrapStatus(): Promise<{ needed: boolean; in_progress: boolean }> {
  // Retry to handle API startup delays after deploy.
  // Total window: ~6 seconds (2 attempts × 3s delay).
  const maxAttempts = 2;
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 2000);
    try {
      const res = await fetch(`${getBase()}/api/v1/auth/bootstrap/status`, {
        signal: controller.signal,
      });
      if (res.ok) {
        return await res.json();
      }
      if (attempt === maxAttempts) break;
    } catch {
      if (attempt === maxAttempts) {
        // All attempts failed — throw so the caller can handle it
        // (e.g. keep retrying instead of showing the wrong UI).
        throw new Error('Cannot connect to the PEPA API. Check that the server is running.');
      }
    } finally {
      clearTimeout(timeoutId);
    }
    // Short delay before retrying
    await new Promise(r => setTimeout(r, 500));
  }
  // Should not reach here, but just in case:
  throw new Error('Cannot determine bootstrap status.');
}

export async function bootstrapActivate(token: string, newPassword: string): Promise<{ token: string; user: { id: string; email: string; name: string; roles: string[] }; must_change_password: boolean }> {
  const res = await fetch(`${getBase()}/api/v1/auth/bootstrap/activate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, new_password: newPassword }),
    credentials: 'include',
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || 'Invalid bootstrap token');
  }
  const data = await res.json();
  // Store the JWT temporarily for cross-origin bootstrap requests.
  // Cross-origin cookies are unreliable; the Authorization header is a
  // safe fallback. Cleared in resetMyPassword() after password change.
  if (data.token) {
    _bootstrapJwt = data.token;
  }
  // Session cookie is also set by the server; store the user profile.
  setStoredUser(data.user);
  return data;
}

// ── User Management API (admin) ──────────────────────────────

export interface User {
  id: string;
  email: string;
  name: string;
  is_active: boolean;
  auth_provider: string;
  is_super_admin: boolean;
  roles: string[];
  last_login_at?: string;
  created_at: string;
}

export async function listUsers(search?: string): Promise<{ users: User[]; total: number }> {
  const q = search ? `?search=${encodeURIComponent(search)}` : '';
  return fetchAPI(`/api/v1/auth/users${q}`);
}

export async function createUser(data: { email: string; name: string; password: string; roles?: string[] }): Promise<{ id: string }> {
  return fetchAPI('/api/v1/auth/users', { method: 'POST', body: JSON.stringify(data) });
}

export async function getUser(id: string): Promise<{ user: User; roles: string[] }> {
  return fetchAPI(`/api/v1/auth/users/${id}`);
}

export async function updateUser(id: string, data: { name?: string; email?: string; is_active?: boolean; roles?: string[] }): Promise<void> {
  await fetchAPI(`/api/v1/auth/users/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deactivateUser(id: string): Promise<void> {
  await fetchAPI(`/api/v1/auth/users/${id}`, { method: 'DELETE' });
}

export async function resetUserPassword(id: string, password: string): Promise<void> {
  await fetchAPI(`/api/v1/auth/users/${id}/reset-password`, { method: 'POST', body: JSON.stringify({ password }) });
}

// ── Teams API ────────────────────────────────────────────────

export interface Team {
  id: string;
  tenant_id: string;
  name: string;
  slug: string;
  description: string;
  parent_team_id?: string;
  created_at: string;
}

export interface TeamMember {
  id: string;
  team_id: string;
  user_id: string;
  email: string;
  name: string;
  role: string;
  joined_at: string;
}

export async function listTeams(): Promise<{ teams: Team[]; total: number }> {
  return fetchAPI('/api/v1/teams');
}

export async function createTeam(data: { name: string; slug: string; description?: string; parent_team_id?: string }): Promise<{ id: string }> {
  return fetchAPI('/api/v1/teams', { method: 'POST', body: JSON.stringify(data) });
}

export async function getTeam(id: string): Promise<{ team: Team; member_count: number }> {
  return fetchAPI(`/api/v1/teams/${id}`);
}

export async function updateTeam(id: string, data: { name?: string; description?: string; parent_team_id?: string | null }): Promise<void> {
  await fetchAPI(`/api/v1/teams/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteTeam(id: string): Promise<void> {
  await fetchAPI(`/api/v1/teams/${id}`, { method: 'DELETE' });
}

export async function listTeamMembers(teamId: string): Promise<{ members: TeamMember[]; total: number }> {
  return fetchAPI(`/api/v1/teams/${teamId}/members`);
}

export async function addTeamMember(teamId: string, userId: string, role?: string): Promise<void> {
  await fetchAPI(`/api/v1/teams/${teamId}/members`, { method: 'POST', body: JSON.stringify({ user_id: userId, role }) });
}

export async function removeTeamMember(teamId: string, userId: string): Promise<void> {
  await fetchAPI(`/api/v1/teams/${teamId}/members/${userId}`, { method: 'DELETE' });
}

export async function getTeamRoles(teamId: string): Promise<{ roles: Array<{ assignment_id: string; role_id: string; name: string; slug: string }> }> {
  return fetchAPI(`/api/v1/teams/${teamId}/roles`);
}

export async function assignTeamRole(teamId: string, roleId: string): Promise<void> {
  await fetchAPI(`/api/v1/teams/${teamId}/roles`, { method: 'POST', body: JSON.stringify({ role_id: roleId }) });
}

export async function removeTeamRole(teamId: string, roleId: string): Promise<void> {
  await fetchAPI(`/api/v1/teams/${teamId}/roles/${roleId}`, { method: 'DELETE' });
}

// ── User Credentials API ─────────────────────────────────────

export interface UserCredential {
  id: string;
  user_id: string;
  provider: string;
  provider_url: string;
  display_name: string;
  token_masked: string;
  username: string;
  email: string;
  is_default: boolean;
  last_verified?: string;
  created_at: string;
}

export async function listMyCredentials(): Promise<{ credentials: UserCredential[]; total: number }> {
  return fetchAPI('/api/v1/my/credentials');
}

export async function createMyCredential(data: { provider: string; provider_url: string; display_name?: string; token: string; username?: string; email?: string; is_default?: boolean }): Promise<{ id: string }> {
  return fetchAPI('/api/v1/my/credentials', { method: 'POST', body: JSON.stringify(data) });
}

export async function updateMyCredential(id: string, data: { display_name?: string; token?: string; username?: string; email?: string; is_default?: boolean }): Promise<void> {
  await fetchAPI(`/api/v1/my/credentials/${id}`, { method: 'PUT', body: JSON.stringify(data) });
}

export async function deleteMyCredential(id: string): Promise<void> {
  await fetchAPI(`/api/v1/my/credentials/${id}`, { method: 'DELETE' });
}

export async function verifyMyCredential(id: string): Promise<{ status: string; message: string }> {
  return fetchAPI(`/api/v1/my/credentials/${id}/verify`, { method: 'POST' });
}

export async function fetchUserInfoForCredential(provider: string, providerUrl: string, token: string, username?: string): Promise<{ username: string; email: string }> {
  return fetchAPI('/api/v1/my/credentials/fetch-user-info', {
    method: 'POST',
    body: JSON.stringify({ provider, provider_url: providerUrl, token, username: username || '' }),
  });
}

// ── Credential Sharing API ───────────────────────────────────

export interface CredentialShareEntry {
  id: string;
  credential_id: string;
  owner_user_id: string;
  shared_with_user?: { id: string; name: string; email: string };
  shared_with_team?: { id: string; name: string };
  created_at: string;
}

export interface SharedCredential {
  id: string;
  owner_name: string;
  owner_email: string;
  provider: string;
  provider_url: string;
  display_name: string;
  token_masked: string;
  username: string;
  email: string;
  created_at: string;
}

export async function listSharedCredentials(): Promise<{ credentials: SharedCredential[]; total: number }> {
  return fetchAPI('/api/v1/my/credentials/shared');
}

export async function shareCredential(credentialId: string, data: { shared_with_user?: string; shared_with_team?: string }): Promise<{ id: string }> {
  return fetchAPI(`/api/v1/my/credentials/${credentialId}/share`, { method: 'POST', body: JSON.stringify(data) });
}

export async function listCredentialShares(credentialId: string): Promise<{ shares: CredentialShareEntry[]; total: number }> {
  return fetchAPI(`/api/v1/my/credentials/${credentialId}/shares`);
}

export async function revokeCredentialShare(credentialId: string, shareId: string): Promise<void> {
  await fetchAPI(`/api/v1/my/credentials/${credentialId}/shares/${shareId}`, { method: 'DELETE' });
}

// ── Entities ────────────────────────────────────────────────

export interface Entity {
  id: string;
  type_id: string;
  type_key: string;
  name: string;
  description?: string;
  external_id?: string;
  tenant_id: string;
  organization_id: string;
  metadata?: Record<string, unknown>;
  status: string;
  sync_status: string;
  created_at: string;
  updated_at: string;
}

export interface EntityListResponse {
  items: Entity[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface EntityType {
  id: string;
  type_key: string;
  display_name: string;
  category: string;
  is_system: boolean;
  is_enabled: boolean;
}

export interface Relationship {
  id: string;
  type_key: string;
  source_id: string;
  target_id: string;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export const entities = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<EntityListResponse>(`/api/v1/entities${qs}`);
  },
  get: (id: string) => fetchAPI<Entity>(`/api/v1/entities/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Entity>('/api/v1/entities', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Entity>(`/api/v1/entities/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/entities/${id}`, { method: 'DELETE' }),
  graph: (id: string, depth = 2) =>
    fetchAPI<{ nodes: unknown[]; edges: unknown[] }>(`/api/v1/entities/${id}/graph?depth=${depth}`),
  relationships: (id: string) =>
    fetchAPI<{ relationships: Relationship[] }>(`/api/v1/entities/${id}/relationships`),
  createRelationship: (id: string, data: { target_id: string; type_key: string }) =>
    fetchAPI<Relationship>(`/api/v1/entities/${id}/relationships`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

export const entityTypes = {
  list: () => fetchAPI<{ entity_types: EntityType[] }>('/api/v1/entity-types'),
};

// ── Workflows ────────────────────────────────────────────────

export interface Workflow {
  id: string;
  name: string;
  tenant_id: string;
  spec: Record<string, unknown>;
  version: number;
  source: string;
  is_enabled: boolean;
  is_locked: boolean;
  created_at: string;
  updated_at: string;
}

export interface WorkflowExecution {
  id: string;
  workflow_id: string;
  tenant_id: string;
  trigger_type: string;
  trigger_payload?: Record<string, unknown>;
  status: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  error?: string;
  result?: Record<string, unknown>;
  created_at: string;
}

export interface StepExecution {
  id: string;
  execution_id: string;
  step_name: string;
  step_type: string;
  plugin_name?: string;
  action_name?: string;
  status: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  output?: Record<string, unknown>;
  error?: string;
  retry_count: number;
}

export const workflows = {
  list: () => fetchAPI<{ workflows: Workflow[]; total: number }>('/api/v1/workflows'),
  get: (id: string) => fetchAPI<Workflow>(`/api/v1/workflows/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Workflow>('/api/v1/workflows', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Workflow>(`/api/v1/workflows/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/workflows/${id}`, { method: 'DELETE' }),
  execute: (id: string, params?: Record<string, unknown>) =>
    fetchAPI<WorkflowExecution>(`/api/v1/workflows/${id}/execute`, {
      method: 'POST',
      body: JSON.stringify({ parameters: params }),
    }),
  executions: (id: string) =>
    fetchAPI<{ executions: WorkflowExecution[] }>(`/api/v1/workflows/${id}/executions`),
  stepExecutions: (execId: string) =>
    fetchAPI<{ step_executions: StepExecution[] }>(`/api/v1/executions/${execId}/steps`),
};

// ── Plugins ────────────────────────────────────────────────

export interface PluginInfo {
  id: string;
  name: string;
  version: string;
  type: string;
  status: string;
  config?: Record<string, unknown>;
  enabled: boolean;
  installed_at: string;
  updated_at: string;
  actions?: string[];
}

export const plugins = {
  list: () => fetchAPI<{ plugins: PluginInfo[] }>('/api/v1/plugins'),
  get: (name: string) => fetchAPI<PluginInfo>(`/api/v1/plugins/${name}`),
  install: (data: { name: string; version: string; type: string; enabled: boolean }) =>
    fetchAPI<PluginInfo>('/api/v1/plugins/install', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  enable: (name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/plugins/${name}/enable`, { method: 'POST' }),
  disable: (name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/plugins/${name}/disable`, { method: 'POST' }),
  configure: (name: string, config: Record<string, unknown>) =>
    fetchAPI<PluginInfo>(`/api/v1/plugins/${name}/configure`, {
      method: 'POST',
      body: JSON.stringify({ config }),
    }),
  uninstall: (name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/plugins/${name}`, { method: 'DELETE' }),
  execute: (name: string, action: string, params?: Record<string, unknown>, config?: Record<string, string>) =>
    fetchAPI<{ success: boolean; output: unknown; error?: string }>(`/api/v1/plugins/${name}/execute`, {
      method: 'POST',
      body: JSON.stringify({ action, params: params || {}, config: config || {} }),
    }),
  health: (name: string) =>
    fetchAPI<{ plugin_name: string; status: string; message?: string; latency?: number }>(`/api/v1/plugins/${name}/health`),
};

// ── Health ────────────────────────────────────────────────

export const health = {
  check: () => fetchAPI<{ status: string; app: string; version: string }>('/healthz'),
};

// ── Scorecards ──────────────────────────────────────────────

export interface Scorecard {
  id: string;
  tenant_id: string;
  name: string;
  description?: string;
  enabled: boolean;
  config?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  rule_count?: number;
}

export interface ScorecardRule {
  id: string;
  scorecard_id: string;
  name: string;
  description?: string;
  expression: string;
  weight: number;
  pass_message?: string;
  fail_message?: string;
  severity: string;
  created_at: string;
}

export interface ScorecardResult {
  id: string;
  scorecard_id: string;
  entity_id: string;
  entity_name?: string;
  score: number;
  max_score: number;
  pass_count: number;
  fail_count: number;
  total_rules: number;
  level: string;
  details: ScorecardRuleResult[];
  evaluated_at: string;
}

export interface ScorecardRuleResult {
  rule_id: string;
  rule_name: string;
  passed: boolean;
  weight: number;
  score: number;
  message: string;
  severity: string;
}

export const scorecards = {
  list: () => fetchAPI<{ scorecards: Scorecard[]; total: number }>('/api/v1/scorecards'),
  get: (id: string) => fetchAPI<{ scorecard: Scorecard; rules: ScorecardRule[] }>(`/api/v1/scorecards/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Scorecard>('/api/v1/scorecards', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Scorecard>(`/api/v1/scorecards/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/scorecards/${id}`, { method: 'DELETE' }),
  addRule: (id: string, data: Record<string, unknown>) =>
    fetchAPI<ScorecardRule>(`/api/v1/scorecards/${id}/rules`, { method: 'POST', body: JSON.stringify(data) }),
  deleteRule: (ruleId: string) =>
    fetchAPI<{ message: string }>(`/api/v1/scorecards/rules/${ruleId}`, { method: 'DELETE' }),
  evaluate: (id: string, entityId: string) =>
    fetchAPI<ScorecardResult>(`/api/v1/scorecards/${id}/evaluate`, {
      method: 'POST',
      body: JSON.stringify({ entity_id: entityId }),
    }),
  evaluateAll: (id: string) =>
    fetchAPI<{ results: ScorecardResult[]; total: number }>(`/api/v1/scorecards/${id}/evaluate`, {
      method: 'POST',
      body: JSON.stringify({ entity_id: 'all' }),
    }),
  results: (id: string) =>
    fetchAPI<{ results: ScorecardResult[]; total: number }>(`/api/v1/scorecards/${id}/results`),
  entityScores: (entityId: string) =>
    fetchAPI<{ scores: ScorecardResult[]; total: number }>(`/api/v1/scorecards/entity/${entityId}`),
};

// ── Audit Log ───────────────────────────────────────────────

export interface AuditEntry {
  id: string;
  tenant_id: string;
  user_id?: string;
  action: string;
  entity_type: string;
  entity_id?: string;
  ip_address?: string;
  user_agent?: string;
  new_values?: Record<string, unknown>;
  old_values?: Record<string, unknown>;
  created_at: string;
}

export interface AuditListResponse {
  items: AuditEntry[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export const audit = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<AuditListResponse>(`/api/v1/audit${qs}`);
  },
  stats: () => fetchAPI<{ by_action: Record<string, number>; by_resource: Record<string, number> }>('/api/v1/audit/stats'),
};

// ── RBAC ──────────────────────────────────────────────────

export interface Role {
  id: string;
  tenant_id: string;
  name: string;
  slug: string;
  description: string;
  is_system: boolean;
  scope: string;
}

export interface Permission {
  id: string;
  role_id: string;
  resource: string;
  action: string;
  effect: string;
}

export interface RoleAssignment {
  id: string;
  tenant_id: string;
  user_id?: string;
  team_id?: string;
  role_id: string;
  role_name: string;
  is_active: boolean;
  user_email?: string;
  user_name?: string;
  team_name?: string;
}

export const rbac = {
  listRoles: () => fetchAPI<{ roles: Role[]; total: number }>('/api/v1/roles'),
  createRole: (data: { name: string; slug: string; description?: string; scope?: string }) =>
    fetchAPI<Role>('/api/v1/roles', { method: 'POST', body: JSON.stringify(data) }),
  updateRole: (id: string, data: { name: string; description?: string; scope?: string }) =>
    fetchAPI<{ message: string }>(`/api/v1/roles/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteRole: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/roles/${id}`, { method: 'DELETE' }),
  getPermissions: (roleId: string) =>
    fetchAPI<{ permissions: Permission[]; total: number }>(`/api/v1/roles/${roleId}/permissions`),
  addPermission: (roleId: string, resource: string, action: string) =>
    fetchAPI<Permission>(`/api/v1/roles/${roleId}/permissions`, {
      method: 'POST',
      body: JSON.stringify({ resource, action }),
    }),
  removePermission: (roleId: string, permId: string) =>
    fetchAPI<{ message: string }>(`/api/v1/roles/${roleId}/permissions/${permId}`, { method: 'DELETE' }),
  listAssignments: () =>
    fetchAPI<{ assignments: RoleAssignment[]; total: number }>('/api/v1/role-assignments'),
  assignRole: (userId: string, roleId: string) =>
    fetchAPI<RoleAssignment>('/api/v1/role-assignments', {
      method: 'POST',
      body: JSON.stringify({ user_id: userId, role_id: roleId }),
    }),
  assignTeamRole: (teamId: string, roleId: string) =>
    fetchAPI<RoleAssignment>('/api/v1/role-assignments', {
      method: 'POST',
      body: JSON.stringify({ team_id: teamId, role_id: roleId }),
    }),
  revokeAssignment: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/role-assignments/${id}`, { method: 'DELETE' }),
  myRoles: () => fetchAPI<{ roles: string[]; assignments: RoleAssignment[] }>('/api/v1/me/roles'),
  check: (resource: string, action: string) =>
    fetchAPI<{ allowed: boolean }>(`/api/v1/me/check?resource=${resource}&action=${action}`),
};

// ── AI Assistant ────────────────────────────────────────────

export interface AIChatRequest {
  message: string;
  conversation_id?: string;
  model?: string;
  enable_tools?: boolean;
  // agent_mode: "native" | "prompt" | "" (auto-detect)
  agent_mode?: string;
  // system_instruction: custom system prompt override
  system_instruction?: string;
  // history: conversation history for multi-turn context
  history?: Array<{ role: string; content: string }>;
  // provider: which LLM provider to use (empty = default)
  provider?: string;
}

export interface AIToolCall {
  tool_name: string;
  tool_args: Record<string, unknown>;
  result?: string;
  error?: string;
  policy: string;
  timestamp: string;
}

export interface AIChatResponse {
  response: string;
  model: string;
  tokens_used: {
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
  };
  tool_calls?: AIToolCall[];
  tools_used?: number;
  needs_approval?: {
    tool_name: string;
    tool_args: Record<string, unknown>;
    description: string;
    reason: string;
  };
}

export interface AITool {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  policy: string;
}

export interface AIStatus {
  providers: Array<{
    name: string;
    available: boolean;
    error?: string;
  }>;
  default_provider: string;
  tools: number;
  enabled: boolean;
  policy: Record<string, string>;
}

export interface AIStreamChunk {
  type: 'text' | 'tool_call' | 'tool_result' | 'error' | 'done';
  content?: string;
  error?: string;
  tool_result?: { content?: string; error?: string };
  metadata?: Record<string, unknown>;
}

export const ai = {
  chat: (req: AIChatRequest) =>
    fetchAPI<AIChatResponse>('/api/v1/ai/chat', {
      method: 'POST',
      body: JSON.stringify(req),
    }),
  /**
   * Stream a chat response via SSE.
   * The onChunk callback is called for each SSE event.
   * Returns a promise that resolves when the stream ends.
   */
  chatStream: async (req: AIChatRequest, onChunk: (chunk: AIStreamChunk) => void, signal?: AbortSignal): Promise<void> => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (_bootstrapJwt) headers['Authorization'] = `Bearer ${_bootstrapJwt}`;
    const res = await fetch(`${getBase()}/api/v1/ai/chat/stream`, {
      method: 'POST',
      headers,
      body: JSON.stringify(req),
      credentials: 'include',
      signal,
    });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      throw new Error(text || `HTTP ${res.status}`);
    }
    const reader = res.body?.getReader();
    if (!reader) throw new Error('Streaming not supported');
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6).trim();
          if (data) {
            try { onChunk(JSON.parse(data) as AIStreamChunk); } catch { /* skip malformed */ }
          }
        }
      }
    }
    // Flush remaining
    if (buffer.startsWith('data: ')) {
      const data = buffer.slice(6).trim();
      if (data) {
        try { onChunk(JSON.parse(data) as AIStreamChunk); } catch { /* skip */ }
      }
    }
  },
  status: () => fetchAPI<AIStatus>('/api/v1/ai/status'),
  tools: () => fetchAPI<{ tools: AITool[]; total: number }>('/api/v1/ai/tools'),
  setDefaultProvider: (provider: string) =>
    fetchAPI<{ default_provider: string; message: string }>('/api/v1/ai/default-provider', {
      method: 'PUT',
      body: JSON.stringify({ provider }),
    }),
};

// ── Clusters ────────────────────────────────────────────────

export interface Cluster {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  environment: string;
  api_server_url: string;
  flux_installed: boolean;
  status: string;
  node_count: number;
  kubernetes_version: string;
  labels: Record<string, string>;
  notes: string;
  is_active: boolean;
  has_kubeconfig: boolean;
  connection_id?: string;
  last_heartbeat_at?: string;
  created_at: string;
  updated_at: string;
}

export interface K8sNamespace {
  name: string;
  status: string;
  pods: number;
}

export interface K8sResource {
  kind: string;
  name: string;
  namespace: string;
  status: string;
  message?: string;
  created_at?: string;
}

export interface FluxResource {
  kind: string;
  name: string;
  namespace: string;
  status: string;
  revision?: string;
  message?: string;
  last_reconciled_at?: string;
  suspended: boolean;
}

export interface ArgoResource {
  kind: string;
  name: string;
  namespace: string;
  status: string;
  health?: string;
  sync_status?: string;
  repo_url?: string;
  target_revision?: string;
  destination?: string;
  message?: string;
  last_updated?: string;
}

export interface ClusterHealth {
  cluster_id: string;
  cluster_name: string;
  status: string;
  api_server: string;
  checks: Record<string, boolean>;
  node_count: number;
  kubernetes_version: string;
  cpu_usage: string;
  memory_usage: string;
  pod_usage: string;
}

export interface ClusterNode {
  name: string;
  status: string;
  roles: string;
  kubernetes_version: string;
  cpu_capacity: string;
  memory_capacity: string;
  pod_capacity: number;
  cpu_usage: string;
  memory_usage: string;
  os_image: string;
  container_runtime: string;
}

export const clusters = {
  list: () => fetchAPI<{ clusters: Cluster[]; total: number }>('/api/v1/clusters'),
  get: (id: string) => fetchAPI<Cluster>(`/api/v1/clusters/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Cluster>('/api/v1/clusters', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Cluster>(`/api/v1/clusters/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/clusters/${id}`, { method: 'DELETE' }),
  uploadKubeconfig: (id: string, kubeconfig: string) =>
    fetchAPI<{ status: string }>(`/api/v1/clusters/${id}/kubeconfig`, {
      method: 'POST',
      body: JSON.stringify({ kubeconfig }),
    }),
  health: (id: string) =>
    fetchAPI<ClusterHealth>(`/api/v1/clusters/${id}/health`),
  nodes: (id: string) =>
    fetchAPI<{ nodes: ClusterNode[]; total: number }>(`/api/v1/clusters/${id}/nodes`),
  namespaces: (id: string) =>
    fetchAPI<{ namespaces: K8sNamespace[] }>(`/api/v1/clusters/${id}/namespaces`),
  resources: (id: string, namespace?: string) => {
    const qs = namespace ? `?namespace=${namespace}` : '';
    return fetchAPI<{ resources: K8sResource[] }>(`/api/v1/clusters/${id}/resources${qs}`);
  },
  fluxResources: (clusterId: string) =>
    fetchAPI<{ resources: FluxResource[] }>(`/api/v1/clusters/${clusterId}/flux`),
  argoResources: (clusterId: string) =>
    fetchAPI<{ resources: ArgoResource[] }>(`/api/v1/clusters/${clusterId}/argo`),
  reconcile: (clusterId: string, namespace: string, name: string, kind: string) =>
    fetchAPI<{ status: string }>(`/api/v1/clusters/${clusterId}/flux/reconcile`, {
      method: 'POST',
      body: JSON.stringify({ namespace, name, kind }),
    }),
  test: (clusterId: string, apiServerUrl?: string) =>
    fetchAPI<{ status: string; message: string; kubernetes_version?: string; node_count?: number; api_server_url?: string }>(`/api/v1/clusters/${clusterId}/test`, {
      method: 'POST',
      body: JSON.stringify({ api_server_url: apiServerUrl || '' }),
    }),
  gitops: (clusterId: string) =>
    fetchAPI<{ fluxcd: boolean; argocd: boolean; flux_count: number; argo_count: number }>(`/api/v1/clusters/${clusterId}/gitops`),
  topology: (clusterId: string) =>
    fetchAPI<GitopsTopologyGraph>(`/api/v1/clusters/${clusterId}/topology`),
};

// ── Deployments ─────────────────────────────────────────────

export interface Deployment {
  id: string;
  tenant_id: string;
  jira_issue_key?: string;
  jira_summary?: string;
  gitlab_project_id?: number;
  gitlab_project_name?: string;
  gitlab_mr_id?: number;
  gitlab_mr_url?: string;
  target_cluster_id?: string;
  target_namespace?: string;
  image_tag?: string;
  image_repository?: string;
  deploy_type?: string;
  replicas?: number;
  strategy?: string;
  spec?: DeploymentSpec;
  status: string;
  error_message?: string;
  logs?: string;
  promoted_by?: string;
  promoted_at?: string;
  created_by?: string;
  timeout_seconds?: number;
  team_name?: string;
  stage?: string;
  created_at: string;
  updated_at: string;
}

export interface DeploymentContainer {
  name: string;
  image: string;
  cpu?: string;
  memory?: string;
  ports?: { containerPort: number; protocol?: string }[];
  env?: Record<string, string>;
  command?: string;
  args?: string;
}

export interface DeploymentSpec {
  containers?: DeploymentContainer[];
  init_containers?: DeploymentContainer[];
  values_yaml?: string;
  service?: { port: number; type: string; targetPort?: number };
  ingress?: { enabled: boolean; host?: string; path?: string };
  health?: { livenessPath?: string; readinessPath?: string; port?: number };
  volumes?: { name: string; type: string; mountPath: string; size?: string }[];
  chart?: { source_type?: string; chart_url?: string; chart_name?: string; chart_version?: string };
}

export const deployments = {
  list: () => fetchAPI<{ deployments: Deployment[]; total: number }>('/api/v1/deployments'),
  get: (id: string) => fetchAPI<Deployment>(`/api/v1/deployments/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Deployment>('/api/v1/deployments', { method: 'POST', body: JSON.stringify(data) }),
  promote: (id: string) =>
    fetchAPI<Deployment>(`/api/v1/deployments/${id}/promote`, { method: 'POST' }),
  rollback: (id: string) =>
    fetchAPI<{ deployment: Deployment; message: string }>(`/api/v1/deployments/${id}/rollback`, { method: 'POST' }),
  cancel: (id: string) =>
    fetchAPI<{ deployment: Deployment; message: string }>(`/api/v1/deployments/${id}/cancel`, { method: 'POST' }),
  history: (id: string) =>
    fetchAPI<{ history: Deployment[]; total: number }>(`/api/v1/deployments/${id}/history`),
  logs: (id: string) =>
    fetchAPI<{ logs: { timestamp: string; level: string; message: string }[]; deployment_id: string; status: string; error_message: string }>(`/api/v1/deployments/${id}/logs`),
  delete: (id: string) =>
    fetchAPI<void>(`/api/v1/deployments/${id}`, { method: 'DELETE' }),
};

// ── Jira Issues ─────────────────────────────────────────────

export interface JiraIssue {
  id: string;
  tenant_id: string;
  issue_key: string;
  issue_id?: string;
  project_key: string;
  summary: string;
  description?: string;
  issue_type: string;
  priority: string;
  status: string;
  assignee?: string;
  reporter?: string;
  labels: string[];
  components: string[];
  fix_versions: string[];
  story_points?: number;
  parent_key?: string;
  jira_url?: string;
  linked_mr_url?: string;
  deployment_id?: string;
  synced_at: string;
  created_at: string;
  updated_at: string;
}

export interface JiraComment {
  id: string;
  tenant_id: string;
  issue_key: string;
  comment_id: string;
  author: string;
  body: string;
  created_at: string;
  updated_at: string;
}

export interface JiraAutomationRule {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  trigger_type: string;
  jira_project_key: string;
  jql_filter: string;
  action_type: string;
  action_config: Record<string, string>;
  enabled: boolean;
  last_triggered_at?: string;
  created_at: string;
  updated_at: string;
}

export interface JiraStats {
  total: number;
  by_status: Record<string, number>;
  by_type: Record<string, number>;
  by_priority: Record<string, number>;
  open_bugs: number;
  by_assignee?: Record<string, number>;
}

export interface JiraAssignee {
  id: string;
  jira_account: string;
  display_name: string;
  email?: string;
  avatar_url?: string;
  active: boolean;
}

export interface JiraSprint {
  id: string;
  jira_id: number;
  board_id: number;
  name: string;
  state: string;
  start_date?: string;
  end_date?: string;
}

export interface JiraWorklog {
  id: string;
  issue_key: string;
  jira_worklog_id: string;
  author: string;
  time_spent: string;
  time_spent_secs: number;
  comment?: string;
  started_at: string;
}

export interface JiraIssueLink {
  id: string;
  inward_key: string;
  outward_key: string;
  link_type: string;
  inward_label?: string;
  outward_label?: string;
}

export interface JiraFilters {
  project_key?: string;
  issue_types?: string[];
  statuses?: string[];
  labels?: string[];
  priorities?: string[];
  assignee?: string;
  search?: string;
  created_from?: string;
  created_to?: string;
  sprint_id?: string;
  components?: string[];
  page?: number;
  page_size?: number;
}

export const jira = {
  list: (filters?: JiraFilters) => {
    const params = new URLSearchParams();
    if (filters) {
      if (filters.project_key) params.set('project_key', filters.project_key);
      if (filters.assignee) params.set('assignee', filters.assignee);
      if (filters.search) params.set('search', filters.search);
      (filters.issue_types || []).forEach(t => params.append('issue_type', t));
      (filters.statuses || []).forEach(s => params.append('status', s));
      (filters.labels || []).forEach(l => params.append('label', l));
      (filters.priorities || []).forEach(p => params.append('priority', p));
    }
    const qs = params.toString();
    return fetchAPI<{ issues: JiraIssue[]; total: number }>(`/api/v1/jira/issues${qs ? '?' + qs : ''}`);
  },
  search: (filters: JiraFilters) =>
    fetchAPI<{ issues: JiraIssue[]; total: number }>('/api/v1/jira/issues/search', {
      method: 'POST',
      body: JSON.stringify(filters),
    }),
  get: (id: string) => fetchAPI<JiraIssue>(`/api/v1/jira/issues/${id}`),
  create: (data: Partial<JiraIssue>) =>
    fetchAPI<JiraIssue>('/api/v1/jira/issues', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id: string, data: Partial<JiraIssue>) =>
    fetchAPI<JiraIssue>(`/api/v1/jira/issues/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/jira/issues/${id}`, { method: 'DELETE' }),
  sync: () =>
    fetchAPI<{ message: string; synced: number }>('/api/v1/jira/sync', { method: 'POST' }),
  linkDeployment: (issueId: string, deploymentId: string) =>
    fetchAPI<JiraIssue>(`/api/v1/jira/issues/${issueId}/link`, {
      method: 'POST',
      body: JSON.stringify({ deployment_id: deploymentId }),
    }),
  getComments: (id: string) =>
    fetchAPI<{ comments: JiraComment[] }>(`/api/v1/jira/issues/${id}/comments`),
  addComment: (id: string, body: string) =>
    fetchAPI<JiraComment>(`/api/v1/jira/issues/${id}/comments`, {
      method: 'POST',
      body: JSON.stringify({ body }),
    }),
  getTransitions: (id: string) =>
    fetchAPI<{ transitions: { id: string; name: string; to: string }[] }>(`/api/v1/jira/issues/${id}/transitions`),
  transition: (id: string, status: string) =>
    fetchAPI<{ issue: JiraIssue; message: string }>(`/api/v1/jira/issues/${id}/transition`, {
      method: 'POST',
      body: JSON.stringify({ status }),
    }),
  getLabels: () =>
    fetchAPI<{ labels: string[] }>('/api/v1/jira/labels'),
  getStats: () =>
    fetchAPI<JiraStats>('/api/v1/jira/stats'),
  getProjects: () =>
    fetchAPI<{ projects: { key: string }[] }>('/api/v1/jira/projects'),
  getAutomationRules: () =>
    fetchAPI<{ rules: JiraAutomationRule[] }>('/api/v1/jira/automation/rules'),
  createAutomationRule: (data: Partial<JiraAutomationRule>) =>
    fetchAPI<JiraAutomationRule>('/api/v1/jira/automation/rules', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAutomationRule: (id: string, data: Partial<JiraAutomationRule>) =>
    fetchAPI<JiraAutomationRule>(`/api/v1/jira/automation/rules/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteAutomationRule: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/jira/automation/rules/${id}`, { method: 'DELETE' }),
  // My issues
  getMyIssues: (assignee?: string, statuses?: string[]) => {
    const params = new URLSearchParams();
    if (assignee) params.set('assignee', assignee);
    (statuses || []).forEach(s => params.append('status', s));
    const qs = params.toString();
    return fetchAPI<{ issues: JiraIssue[]; total: number }>(`/api/v1/jira/my-issues${qs ? '?' + qs : ''}`);
  },
  // Assignees
  getAssignees: () =>
    fetchAPI<{ assignees: JiraAssignee[] }>('/api/v1/jira/assignees'),
  // Sprints
  getSprints: (state?: string) => {
    const qs = state ? `?state=${state}` : '';
    return fetchAPI<{ sprints: JiraSprint[] }>(`/api/v1/jira/sprints${qs}`);
  },
  // Components
  getComponents: () =>
    fetchAPI<{ components: string[] }>('/api/v1/jira/components'),
  // Worklogs
  getWorklogs: (id: string) =>
    fetchAPI<{ worklogs: JiraWorklog[]; total_seconds: number }>(`/api/v1/jira/issues/${id}/worklogs`),
  addWorklog: (id: string, data: { time_spent: string; time_spent_secs?: number; comment?: string }) =>
    fetchAPI<JiraWorklog>(`/api/v1/jira/issues/${id}/worklogs`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // Issue Links
  getIssueLinks: (id: string) =>
    fetchAPI<{ links: JiraIssueLink[] }>(`/api/v1/jira/issues/${id}/links`),
  // Create in remote Jira (via plugin)
  createInJira: (data: {
    project_key: string;
    summary: string;
    issue_type: string;
    description?: string;
    priority?: string;
    assignee?: string;
    labels?: string[];
    parent_key?: string;
    epic_link?: string;
    linked_issue_key?: string;
    link_type?: string;
  }) =>
    fetchAPI<{ issue_key: string; summary: string; status: string; issue: JiraIssue }>('/api/v1/jira/create', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

// ── Connections ─────────────────────────────────────────────

export type ConnectionType = 'kubernetes' | 'gitlab' | 'git' | 'jira' | 'ci' | 'ai' | 'storage' | 'proxmox' | 'vmware' | 'notification' | 'docker' | 'secret' | 'sonarqube';

export interface Connection {
  id: string;
  tenant_id: string;
  type: ConnectionType;
  name: string;
  description: string;
  config: Record<string, unknown>;
  status: string;
  last_check_at?: string;
  labels: Record<string, string>;
  notes: string;
  created_at: string;
  updated_at: string;
}

export interface ConnectionSummary {
  type: ConnectionType;
  count: number;
}

export interface ParsedCluster {
  name: string;
  server: string;
  kubeconfig: string;
}

export const connections = {
  list: (type?: string) => {
    const qs = type ? `?type=${type}` : '';
    return fetchAPI<{ connections: Connection[]; total: number }>(`/api/v1/connections${qs}`);
  },
  get: (id: string) => fetchAPI<Connection>(`/api/v1/connections/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Connection>('/api/v1/connections', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Connection>(`/api/v1/connections/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/connections/${id}`, { method: 'DELETE' }),
  test: (id: string) =>
    fetchAPI<{ status: string; message: string; type: string; name: string }>(`/api/v1/connections/${id}/test`, { method: 'POST' }),
  browse: (id: string, resource?: string, params?: Record<string, string>) => {
    const qs = new URLSearchParams();
    if (resource) qs.set('resource', resource);
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        if (v) qs.set(k, v);
      }
    }
    const query = qs.toString();
    return fetchAPI<{ resource: string; data: unknown }>(`/api/v1/connections/${id}/browse${query ? `?${query}` : ''}`);
  },
  execute: (id: string, resource: string, params?: Record<string, unknown>) =>
    fetchAPI<{ resource: string; data: unknown }>(`/api/v1/connections/${id}/execute`, {
      method: 'POST',
      body: JSON.stringify({ resource, params }),
    }),
  summary: () =>
    fetchAPI<{ summary: ConnectionSummary[] }>('/api/v1/connections/summary'),
  pluginStatus: () =>
    fetchAPI<Record<string, { installed: boolean; enabled: boolean }>>('/api/v1/connections/plugin-status'),
  parseKubeconfig: (kubeconfig: string) =>
    fetchAPI<{ clusters: ParsedCluster[]; count: number }>('/api/v1/connections/parse-kubeconfig', {
      method: 'POST',
      body: JSON.stringify({ kubeconfig }),
    }),
};

// ── Git Browser ──────────────────────────────────────────────

export interface GitGroup {
  id: string;
  name: string;
  full_name: string;
  url?: string;
  kind: string; // "group", "organization", "workspace"
}

export interface GitRepo {
  id: string;
  name: string;
  full_name: string;
  description?: string;
  url: string;
  default_branch: string;
  private: boolean;
}

export interface GitPipeline {
  id: number;
  sha: string;
  ref: string;
  status: string;
  source?: string;
  url?: string;
}

export interface GitPipelineJob {
  id: number;
  name: string;
  stage: string;
  status: string;
  ref: string;
  allow_failure: boolean;
  duration: number;
  runner: string;
  web_url: string;
  started_at?: string;
  finished_at?: string;
}

export interface GitPipelineTriggerResult {
  id: number;
  sha: string;
  ref: string;
  status: string;
  url: string;
}

export interface CIVariable {
  key: string;
  value: string;
  description: string;
  type: string; // "env_var" | "file"
  options?: string[]; // enum options for choice-type variables
  required?: boolean;
  is_input?: boolean; // true for spec.inputs (component pipeline inputs)
}

export interface WorkflowInfo {
  file: string;
  name: string;
  triggers: string[];
  jobs: string[];
  has_dispatch: boolean;
}

export const gitBrowser = {
  listGroups: (connectionId: string, parentId?: string) =>
    connections.browse(connectionId, 'list_groups', parentId ? { parent_id: parentId } : undefined)
      .then(r => ({ groups: ((r.data as any)?.groups || []) as GitGroup[], total: ((r.data as any)?.total || 0) as number })),
  listRepos: (connectionId: string, groupId?: string) =>
    connections.browse(connectionId, 'list_repos', groupId ? { group_id: groupId } : undefined)
      .then(r => ({ repos: ((r.data as any)?.repos || []) as GitRepo[], total: ((r.data as any)?.total || 0) as number })),
  listPipelines: (connectionId: string, repoId: string) =>
    connections.browse(connectionId, 'list_pipelines', { repo_id: repoId })
      .then(r => ({ pipelines: ((r.data as any)?.pipelines || []) as GitPipeline[], total: ((r.data as any)?.total || 0) as number })),
  listBranches: (connectionId: string, repoId: string) =>
    connections.browse(connectionId, 'get_branches', { repo_id: repoId })
      .then(r => ({ branches: ((r.data as any)?.branches || []) as { name: string; sha: string; protected: boolean }[], total: ((r.data as any)?.total || 0) as number })),
  triggerPipeline: (connectionId: string, repoId: string, ref: string, variables?: Record<string, string>, inputs?: Record<string, string>) =>
    connections.execute(connectionId, 'trigger_pipeline', { repo_id: repoId, ref, variables, inputs })
      .then(r => (r.data as any) as GitPipelineTriggerResult),
  getPipelineJobs: (connectionId: string, repoId: string, pipelineId: number) =>
    connections.execute(connectionId, 'get_pipeline_jobs', { repo_id: repoId, pipeline_id: pipelineId })
      .then(r => ({ jobs: ((r.data as any)?.jobs || []) as GitPipelineJob[], total: ((r.data as any)?.total || 0) as number })),
  getJobLog: (connectionId: string, repoId: string, jobId: number) =>
    connections.execute(connectionId, 'get_job_log', { repo_id: repoId, job_id: jobId })
      .then(r => ({ log: ((r.data as any)?.log || '') as string, job_id: ((r.data as any)?.job_id || jobId) as number })),
  parseCIConfig: (connectionId: string, repoId: string, ref?: string) =>
    connections.execute(connectionId, 'parse_ci_config', { repo_id: repoId, ref: ref || '' })
      .then(r => ({
        variables: ((r.data as any)?.variables || []) as CIVariable[],
        has_ci_file: ((r.data as any)?.has_ci_file || false) as boolean,
        workflows: ((r.data as any)?.workflows || []) as WorkflowInfo[],
      })),
};

// ── Service Templates & Services ────────────────────────────

export interface ServiceTemplateHelmChart {
  repo_url?: string;
  chart_name?: string;
  chart_version?: string;
  image?: string;
  docs_url?: string;
}

export interface ServiceTemplate {
  id: string;
  tenant_id: string;
  name: string;
  slug: string;
  description: string;
  category: string;
  icon?: string;
  language?: string;
  framework?: string;
  tags: string[];
  helm_chart?: ServiceTemplateHelmChart;
  resource_defaults?: Record<string, unknown>;
  default_values?: Record<string, string>;
  is_enabled: boolean;
  is_system: boolean;
  created_at: string;
}

export interface Service {
  id: string;
  tenant_id: string;
  template_id?: string;
  name: string;
  slug: string;
  description: string;
  language?: string;
  framework?: string;
  gitlab_project_url?: string;
  helm_chart_url?: string;
  image_repository?: string;
  namespace: string;
  status: string;
  resource_config?: Record<string, unknown>;
  environment_variables?: Record<string, string>;
  deployment_strategy: string;
  target_clusters: string[];
  created_at: string;
  updated_at: string;
}

export interface ServiceDeployment {
  id: string;
  tenant_id: string;
  service_id: string;
  environment: string;
  cluster_id?: string;
  namespace?: string;
  branch?: string;
  image_tag?: string;
  helm_release?: string;
  deploy_type: string;
  status: string;
  verification_status: string;
  flux_synced: boolean;
  pods_ready: number;
  pods_total: number;
  mr_url?: string;
  pipeline_url?: string;
  deployed_at?: string;
  verified_at?: string;
  promoted_at?: string;
  created_at: string;
}

export const serviceTemplates = {
  list: () =>
    fetchAPI<{ templates: ServiceTemplate[]; total: number }>('/api/v1/service-templates'),
  get: (slug: string) =>
    fetchAPI<ServiceTemplate>(`/api/v1/service-templates/${slug}`),
};

export const services = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ items: Service[]; total: number; page: number; per_page: number; total_pages: number }>(`/api/v1/services${qs}`);
  },
  get: (id: string) =>
    fetchAPI<{ service: Service; deployments: ServiceDeployment[] }>(`/api/v1/services/${id}`),
  create: (data: Record<string, unknown>) =>
    fetchAPI<Service>('/api/v1/services', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Service>(`/api/v1/services/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/services/${id}`, { method: 'DELETE' }),
  deploy: (id: string, data: { environment: string; cluster_id?: string; branch?: string; image_tag?: string; deploy_type?: string }) =>
    fetchAPI<ServiceDeployment>(`/api/v1/services/${id}/deploy`, { method: 'POST', body: JSON.stringify(data) }),
  deployments: (id: string) =>
    fetchAPI<{ deployments: ServiceDeployment[]; total: number }>(`/api/v1/services/${id}/deployments`),
  verify: (serviceId: string, deploymentId: string) =>
    fetchAPI<{ deployment: ServiceDeployment; message: string }>(`/api/v1/services/${serviceId}/deployments/${deploymentId}/verify`, { method: 'POST' }),
  promote: (serviceId: string, deploymentId: string) =>
    fetchAPI<{ message: string }>(`/api/v1/services/${serviceId}/deployments/${deploymentId}/promote`, { method: 'POST' }),
};

// ── Catalog ─────────────────────────────────────────────────

export interface CatalogItem {
  id: string;
  name: string;
  slug: string;
  description: string;
  language: string;
  framework: string;
  namespace: string;
  status: string;
  deployment_strategy: string;
  template_name: string;
  category: string;
  tags: string[];
  deployment_count: number;
  active_environments: string[];
  created_at: string;
  updated_at: string;
}

export interface CatalogHealth {
  service_id: string;
  service_name: string;
  status: string;
  total_envs: number;
  healthy_envs: number;
  total_pods: number;
  ready_pods: number;
  deployments: number;
}

export const catalog = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ items: CatalogItem[]; total: number }>(`/api/v1/catalog${qs}`);
  },
  get: (id: string) =>
    fetchAPI<{ service: Service; deployments: ServiceDeployment[]; template_name: string; category: string; tags: string[] }>(`/api/v1/catalog/${id}`),
  health: (id: string) =>
    fetchAPI<CatalogHealth>(`/api/v1/catalog/${id}/health`),
  metrics: (id: string) =>
    fetchAPI<{ service_id: string; resources: Record<string, unknown>; cpu_request: string; memory_request: string; replicas_desired: number }>(`/api/v1/catalog/${id}/metrics`),
};

// ── GitOps Workflow ─────────────────────────────────────────

export interface WorkflowMR {
  id: string;
  jira_issue_key: string;
  jira_summary: string;
  project_name: string;
  mr_id: number;
  mr_url: string;
  image_tag: string;
  image_repository: string;
  status: string;
  stage: string;
  team: string;
  namespace: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowConfig {
  pipeline: {
    stages: Array<{
      name: string;
      label: string;
      auto_deploy: boolean;
      verification: boolean;
    }>;
  };
  gitops: {
    provider: string;
    repo_url: string;
    branch: string;
    path_pattern: string;
  };
  ci: {
    provider: string;
    trigger: string;
    image_tag: string;
  };
  verification: {
    checks: string[];
    timeout: string;
  };
}

export interface TimelineEvent {
  timestamp: string;
  stage: string;
  status: string;
  label: string;
  detail: string;
}

export const gitops = {
  config: () =>
    fetchAPI<WorkflowConfig>('/api/v1/gitops/config'),
  mrs: (team?: string) =>
    fetchAPI<{ merge_requests: WorkflowMR[]; total: number }>(`/api/v1/gitops/mrs${team ? `?team=${encodeURIComponent(team)}` : ''}`),
  deploy: (data: { jira_issue_key?: string; jira_summary?: string; project_name?: string; image_tag: string; image_repository?: string; namespace?: string; cluster_id?: string; team?: string; stage?: string }) =>
    fetchAPI<{ deployment: Deployment; message: string }>('/api/v1/gitops/deploy', { method: 'POST', body: JSON.stringify(data) }),
  promote: (deploymentId: string) =>
    fetchAPI<{ deployment?: Deployment; awaiting_approval: boolean; target_stage?: string; promoted_to?: string; new_deployment?: Deployment; message?: string }>(`/api/v1/gitops/deployments/${deploymentId}/promote`, { method: 'POST' }),
  approve: (deploymentId: string) =>
    fetchAPI<{ deployment?: Deployment; promoted_to: string; message?: string }>(`/api/v1/gitops/deployments/${deploymentId}/approve`, { method: 'POST' }),
  rollback: (deploymentId: string) =>
    fetchAPI<{ deployment?: Deployment; message?: string }>(`/api/v1/gitops/deployments/${deploymentId}/rollback`, { method: 'POST' }),
  verify: (deploymentId: string) =>
    fetchAPI<{ deployment_id: string; verification_status: string; checks: Array<{ name: string; status: string; message: string }>; verified_at: string }>('/api/v1/gitops/verify', { method: 'POST', body: JSON.stringify({ deployment_id: deploymentId }) }),
  timeline: (id: string) =>
    fetchAPI<{ deployment_id: string; events: TimelineEvent[]; history: Array<Record<string, unknown>>; total: number }>(`/api/v1/gitops/timeline/${id}`),
  // Manifest repository management
  listRepos: () =>
    fetchAPI<{ repos: GitopsRepo[]; total: number }>('/api/v1/gitops/repos'),
  getRepo: (id: string) =>
    fetchAPI<GitopsRepo>(`/api/v1/gitops/repos/${id}`),
  createRepo: (data: { name: string; repo_url: string; branch?: string; path?: string; engine_type?: string; connection_id?: string; token?: string; argocd_server_url?: string; argocd_auth_token?: string }) =>
    fetchAPI<GitopsRepo>('/api/v1/gitops/repos', { method: 'POST', body: JSON.stringify(data) }),
  updateRepo: (id: string, data: { name?: string; repo_url?: string; branch?: string; path?: string; engine_type?: string; token?: string; connection_id?: string; argocd_server_url?: string; argocd_auth_token?: string }) =>
    fetchAPI<GitopsRepo>(`/api/v1/gitops/repos/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteRepo: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/gitops/repos/${id}`, { method: 'DELETE' }),
  scanRepo: (id: string) =>
    fetchAPI<{ message: string; resources: GitopsResource[]; engine: string; file_count: number; total: number }>(`/api/v1/gitops/repos/${id}/scan`, { method: 'POST' }),
  listResources: (id: string, params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ resources: GitopsResource[]; total: number; engine: string; scan_status: string; tree?: GitopsFileNode; clusters?: GitopsClusterInfo[]; layout?: GitopsLayout }>(`/api/v1/gitops/repos/${id}/resources${qs}`);
  },
  listClusters: (id: string) =>
    fetchAPI<{ clusters: string[]; total: number }>(`/api/v1/gitops/repos/${id}/clusters`),
  createResource: (id: string, data: { kind: string; name: string; namespace: string; cluster?: string; chart?: string; version?: string; source_ref?: string; values?: Record<string, unknown>; commit_message?: string }) =>
    fetchAPI<{ resource: GitopsResource; commit: { commit_sha: string; branch: string; mr_needed: boolean } }>(`/api/v1/gitops/repos/${id}/resources`, { method: 'POST', body: JSON.stringify(data) }),
  // Editing
  editValues: (repoId: string, resourceId: string, data: { file_path: string; field_path?: string; new_value?: unknown; full_yaml?: string; commit_message?: string; branch?: string }) =>
    fetchAPI<{ commit_sha: string; branch: string; mr_url?: string; mr_needed: boolean; diff: string }>(`/api/v1/gitops/repos/${repoId}/resources/${resourceId}/values`, { method: 'PUT', body: JSON.stringify(data) }),
  previewDiff: (repoId: string, resourceId: string, data: { file_path: string; field_path?: string; new_value?: unknown; full_yaml?: string }) =>
    fetchAPI<{ diff: string }>(`/api/v1/gitops/repos/${repoId}/resources/${resourceId}/values/preview`, { method: 'POST', body: JSON.stringify(data) }),
  suggestCommitMessage: (repoId: string, resourceId: string, data: { resource_kind: string; resource_name: string; file_path: string; changes?: string }) =>
    fetchAPI<{ suggested_message: string; prefix: string }>(`/api/v1/gitops/repos/${repoId}/resources/${resourceId}/values/suggest-commit`, { method: 'POST', body: JSON.stringify(data) }),
  suspendResource: (repoId: string, resourceId: string, data: { file_path: string; suspend: boolean; commit_message?: string; resource_kind?: string; resource_name?: string }) =>
    fetchAPI<{ commit_sha: string; branch: string; mr_needed: boolean; diff: string }>(`/api/v1/gitops/repos/${repoId}/resources/${resourceId}/suspend`, { method: 'POST', body: JSON.stringify(data) }),
  // Topology
  topology: (id: string) =>
    fetchAPI<GitopsTopologyGraph>(`/api/v1/gitops/repos/${id}/topology`),
  // Drift detection
  detectDrift: (id: string, clusterId?: string, path?: string) => {
    const params = new URLSearchParams();
    if (clusterId) params.set('cluster_id', clusterId);
    if (path) params.set('path', path);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return fetchAPI<GitopsDriftResult>(`/api/v1/gitops/repos/${id}/drift${qs}`);
  },
  // Live cluster status for resources
  liveStatus: (id: string) =>
    fetchAPI<{ status_map: Record<string, { health: string; sync_status: string; revision: string; cluster: string }>; total: number }>(`/api/v1/gitops/repos/${id}/live-status`),
  // List overlay paths in a repo
  listOverlays: (id: string) =>
    fetchAPI<{ overlays: string[] }>(`/api/v1/gitops/repos/${id}/overlays`),
  // Per-repo cluster & scope mapping for drift detection
  updateMapping: (id: string, data: { cluster_id: string; scope_path?: string }) =>
    fetchAPI<{ cluster_id: string; scope_path: string }>(`/api/v1/gitops/repos/${id}/mapping`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteMapping: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/gitops/repos/${id}/mapping`, { method: 'DELETE' }),
  // Tracking (SSE URL helper — use EventSource on client)
  trackURL: (repoId: string, commitSHA: string) =>
    `${getBase()}/api/v1/gitops/repos/${repoId}/track/${commitSHA}`,
};

// ── Vault ───────────────────────────────────────────────────

export interface VaultPath {
  path: string;
  type: string;
  has_children: boolean;
  keys?: string[];
}

export interface VaultEngine {
  path: string;
  type: string;
  description: string;
  version: string;
}

export interface VaultConfig {
  mode: 'local' | 'remote';
  address?: string;
  token?: string;
  mount_path?: string;
}

export interface VaultStatus {
  total_secrets: number;
  v1_secrets: number;
  v2_secrets: number;
  encryption_type: string;
  key_derivation: string;
  per_path_keys: boolean;
  needs_rotation: boolean;
  tenant_isolation: boolean;
  created_by_tracking: boolean;
  argon2_params: { time: number; memory: number; threads: number; keyLen: number };
}

export const vault = {
  paths: (prefix?: string) => {
    const qs = prefix ? `?prefix=${encodeURIComponent(prefix)}` : '';
    return fetchAPI<{ paths: VaultPath[]; total: number; mode: string }>(`/api/v1/vault/paths${qs}`);
  },
  getSecret: (path: string) =>
    fetchAPI<{ path: string; secret: { data: Record<string, string>; metadata: Record<string, unknown> }; mode: string }>(`/api/v1/vault/secrets/${path}`),
  setSecret: (path: string, data: Record<string, string>) =>
    fetchAPI<{ path: string; status: string; version: number; mode: string }>(`/api/v1/vault/secrets/${path}`, { method: 'POST', body: JSON.stringify({ data }) }),
  engines: () =>
    fetchAPI<{ engines: VaultEngine[]; total: number; mode: string }>('/api/v1/vault/engines'),
  getConfig: () =>
    fetchAPI<{ config: VaultConfig }>('/api/v1/vault/config'),
  saveConfig: (config: VaultConfig) =>
    fetchAPI<{ message: string; mode: string }>('/api/v1/vault/config', { method: 'POST', body: JSON.stringify(config) }),
  testConnection: (address: string, token?: string, mountPath?: string) =>
    fetchAPI<{ status: string; message: string; type: string; name: string; details: Record<string, unknown> }>('/api/v1/vault/test-connection', { method: 'POST', body: JSON.stringify({ address, token, mount_path: mountPath }) }),
  getStatus: () =>
    fetchAPI<{ status: VaultStatus; mode: string }>('/api/v1/vault/status'),
  rotateKeys: () =>
    fetchAPI<{ rotated: number; errors: string[]; message: string }>('/api/v1/vault/rotate', { method: 'POST' }),
  // ACL management
  listACL: () =>
    fetchAPI<{ entries: VaultACLEntry[]; total: number }>('/api/v1/vault/acl'),
  createACL: (entry: { path_prefix: string; user_id?: string; team_id?: string; can_read?: boolean; can_create?: boolean; can_delete?: boolean }) =>
    fetchAPI<{ id: string; message: string }>('/api/v1/vault/acl', { method: 'POST', body: JSON.stringify(entry) }),
  deleteACL: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/vault/acl/${id}`, { method: 'DELETE' }),
};

export interface VaultACLEntry {
  id: string;
  path_prefix: string;
  user_id?: string;
  team_id?: string;
  user_name?: string;
  team_name?: string;
  can_read: boolean;
  can_create: boolean;
  can_delete: boolean;
  created_by?: string;
  created_at: string;
}

// ── Import ──────────────────────────────────────────────────

export interface GitLabGroup {
  id: number;
  name: string;
  full_path: string;
  description: string;
  visibility: string;
  project_count: number;
  subgroup_count: number;
  web_url: string;
}

export interface GitLabProject {
  id: number;
  name: string;
  path_with_namespace: string;
  description: string;
  default_branch: string;
  has_helm_chart: boolean;
  last_activity: string;
}

export interface HelmChart {
  name: string;
  version: string;
  app_version: string;
  description: string;
  path: string;
  values_path: string;
  project_id: string;
}

export const gitlabImport = {
  groups: () =>
    fetchAPI<{ groups: GitLabGroup[]; total: number }>('/api/v1/import/gitlab/groups'),
  projects: (groupId: string) =>
    fetchAPI<{ projects: GitLabProject[]; total: number }>(`/api/v1/import/gitlab/groups/${groupId}/projects`),
  charts: (projectId: string) =>
    fetchAPI<{ charts: HelmChart[]; total: number }>(`/api/v1/import/gitlab/projects/${projectId}/charts`),
  importChart: (data: { project_id: number; project_name: string; chart_name: string; chart_path?: string; version?: string; namespace?: string }) =>
    fetchAPI<{ service: Record<string, unknown>; message: string }>('/api/v1/import/gitlab/charts', { method: 'POST', body: JSON.stringify(data) }),
  mr: (mrId: string) =>
    fetchAPI<Record<string, unknown>>(`/api/v1/import/gitlab/mr/${mrId}`),
  deployMR: (mrId: string, data: { environment: string; cluster_id?: string; namespace?: string }) =>
    fetchAPI<{ deployment: Record<string, unknown>; message: string }>(`/api/v1/import/gitlab/mr/${mrId}/deploy`, { method: 'POST', body: JSON.stringify(data) }),
};

// ── Policies (Phase 2.1) ─────────────────────────────────────

export interface Policy {
  id: string;
  name: string;
  description: string;
  category: string;
  severity: string;
  enabled: boolean;
  policy_code: string;
  template: string;
  violations: number;
  last_evaluated: string;
  created_at: string;
  updated_at: string;
}

export interface PolicyViolation {
  id: string;
  policy_id: string;
  policy_name: string;
  resource: string;
  namespace: string;
  cluster: string;
  message: string;
  severity: string;
  status: string;
  timestamp: string;
}

export interface PolicyTemplate {
  id: string;
  name: string;
  description: string;
  category: string;
  severity: string;
}

export const policies = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ policies: Policy[]; total: number; summary: Record<string, unknown> }>(`/api/v1/policies${qs}`);
  },
  get: (id: string) =>
    fetchAPI<{ policy: Policy; violations: PolicyViolation[] }>(`/api/v1/policies/${id}`),
  create: (data: { name: string; description?: string; category: string; severity: string; policy_code: string; enabled?: boolean }) =>
    fetchAPI<{ policy: Policy; message: string }>('/api/v1/policies', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<{ policy: Policy; message: string }>(`/api/v1/policies/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/policies/${id}`, { method: 'DELETE' }),
  validate: (policyCode: string) =>
    fetchAPI<{ valid: boolean; errors: string[]; warnings: string[]; message: string }>('/api/v1/policies/validate', { method: 'POST', body: JSON.stringify({ policy_code: policyCode }) }),
  audit: (id: string) =>
    fetchAPI<{ policy_id: string; events: Array<Record<string, unknown>>; total: number }>(`/api/v1/policies/${id}/audit`),
  templates: () =>
    fetchAPI<{ templates: PolicyTemplate[]; total: number }>('/api/v1/policy-templates'),
  violations: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ violations: PolicyViolation[]; total: number; summary: Record<string, unknown> }>(`/api/v1/policy-violations${qs}`);
  },
  resolveViolation: (id: string) =>
    fetchAPI<{ violation: PolicyViolation; message: string }>(`/api/v1/policy-violations/${id}/resolve`, { method: 'POST' }),
};

// ── Security (Phase 2.2) ─────────────────────────────────────

export interface ScanResult {
  id: string;
  image: string;
  status: string;
  severity: { critical: number; high: number; medium: number; low: number; unknown: number };
  scan_type: string;
  duration_ms: number;
  scanned_at: string;
}

export interface Vulnerability {
  id: string;
  scan_id: string;
  cve: string;
  title: string;
  description: string;
  severity: string;
  package: string;
  version: string;
  fixed_in: string;
  status: string;
  references: string;
}

export const security = {
  dashboard: () =>
    fetchAPI<{ overview: Record<string, unknown>; recent_scans: ScanResult[]; critical_vulns: Vulnerability[]; scan_trend: Array<Record<string, unknown>> }>('/api/v1/security/dashboard'),
  scans: () =>
    fetchAPI<{ scans: ScanResult[]; total: number; summary: Record<string, unknown> }>('/api/v1/security/scans'),
  getScan: (id: string) =>
    fetchAPI<{ scan: ScanResult; vulnerabilities: Vulnerability[] }>(`/api/v1/security/scans/${id}`),
  triggerScan: (image: string, scanType?: string) =>
    fetchAPI<{ scan: ScanResult; message: string }>('/api/v1/security/scans', { method: 'POST', body: JSON.stringify({ image, scan_type: scanType || 'image' }) }),
  vulnerabilities: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ vulnerabilities: Vulnerability[]; total: number; summary: Record<string, unknown> }>(`/api/v1/security/vulnerabilities${qs}`);
  },
  fixVulnerability: (id: string) =>
    fetchAPI<{ vulnerability: Vulnerability; message: string }>(`/api/v1/security/vulnerabilities/${id}/fix`, { method: 'POST' }),
};

// ── SonarQube Code Quality ────────────────────────────────────

export interface SonarQubeQualityGate {
  status: string; // OK, ERROR, WARN, NONE
  conditions: Array<{
    status: string;
    metric_key: string;
    comparator: string;
    period_index: number;
    error_threshold: string;
    actual_value: string;
  }>;
}

export interface SonarQubeIssue {
  key: string;
  rule: string;
  severity: string;
  status: string;
  message: string;
  component: string;
  project: string;
  line?: number;
  type: string; // BUG, VULNERABILITY, CODE_SMELL
  tags?: string[];
  creation_date: string;
  update_date: string;
}

export interface SonarQubeMeasure {
  metric: string;
  value: string;
  best_value?: boolean;
}

export interface SonarQubeMeasures {
  component: string;
  branch?: string;
  metrics: SonarQubeMeasure[];
}

export interface SonarQubeIssueSummary {
  bugs: number;
  vulnerabilities: number;
  code_smells: number;
  by_severity: Record<string, number>;
}

export interface SonarQubeProjectSummary {
  project_key: string;
  branch: string;
  quality_gate?: SonarQubeQualityGate;
  measures?: SonarQubeMeasures;
  issue_summary?: SonarQubeIssueSummary;
  fetched_at: string;
}

export const sonarqube = {
  /** Execute a SonarQube plugin action via the plugin execute endpoint. */
  execute: (action: string, params?: Record<string, unknown>) =>
    plugins.execute('sonarqube', action, params),

  getProjectSummary: (projectKey?: string) =>
    sonarqube.execute('get_project_summary', projectKey ? { project_key: projectKey } : undefined),

  getQualityGate: (projectKey?: string) =>
    sonarqube.execute('get_quality_gate', projectKey ? { project_key: projectKey } : undefined),

  getIssues: (params?: { project_key?: string; types?: string; severities?: string; page_size?: number }) =>
    sonarqube.execute('get_issues', params),

  getCoverage: (projectKey?: string) =>
    sonarqube.execute('get_coverage', projectKey ? { project_key: projectKey } : undefined),

  getMeasures: (projectKey?: string, metricKeys?: string) =>
    sonarqube.execute('get_measures', {
      ...(projectKey ? { project_key: projectKey } : {}),
      ...(metricKeys ? { metric_keys: metricKeys } : {}),
    }),
};

// ── Environments (Phase 2.4) ─────────────────────────────────

export interface Environment {
  id: string;
  tenant_id?: string;
  name: string;
  slug?: string;
  type?: string;
  cluster?: string;
  namespace?: string;
  status?: string;
  description: string;
  color?: string;
  is_default?: boolean;
  variables?: EnvVariable[];
  created_at: string;
  updated_at: string;
}

export interface EnvVariable {
  id: string;
  env_id: string;
  key: string;
  value: string;
  is_secret: boolean;
  source: string;
  created_at: string;
  updated_at: string;
}

export interface EnvironmentContents {
  environment: Environment;
  clusters: Array<{
    id: string;
    name: string;
    description: string;
    status: string;
    kubernetes_version: string;
    node_count: number;
  }>;
  deployments: Array<{
    id: string;
    service_id: string;
    environment: string;
    status: string;
    deploy_type: string;
    image_tag: string;
    pods_ready: number;
    pods_total: number;
    created_at: string;
  }>;
  variables_count: number;
  summary: {
    cluster_count: number;
    deployment_count: number;
    variable_count: number;
  };
}

export interface EnvCompareEntry {
  key: string;
  value1: string;
  value2: string;
  status: 'same' | 'different' | 'only_env1' | 'only_env2';
  is_secret: boolean;
}

export const environments = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ environments: Environment[]; total: number }>(`/api/v1/environments${qs}`);
  },
  get: (id: string) =>
    fetchAPI<Environment>(`/api/v1/environments/${id}`),
  create: (data: { name: string; type?: string; slug?: string; cluster?: string; namespace?: string; status?: string; description?: string; color?: string; is_default?: boolean }) =>
    fetchAPI<Environment>('/api/v1/environments', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<Environment>(`/api/v1/environments/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/environments/${id}`, { method: 'DELETE' }),
  contents: (id: string) =>
    fetchAPI<EnvironmentContents>(`/api/v1/environments/${id}/contents`),
  variables: (id: string) =>
    fetchAPI<{ variables: EnvVariable[]; total: number }>(`/api/v1/environments/${id}/variables`),
  setVariable: (id: string, data: { key: string; value: string; is_secret?: boolean; source?: string }) =>
    fetchAPI<{ variable: EnvVariable; message: string }>(`/api/v1/environments/${id}/variables`, { method: 'POST', body: JSON.stringify(data) }),
  deleteVariable: (id: string, key: string) =>
    fetchAPI<{ message: string }>(`/api/v1/environments/${id}/variables/${key}`, { method: 'DELETE' }),
  compare: (env1: string, env2: string) =>
    fetchAPI<{ env1: string; env2: string; comparison: EnvCompareEntry[]; total: number; differences: number }>(`/api/v1/environments/compare?env1=${env1}&env2=${env2}`),
};

// ── Integrations (Phase 2.5) ─────────────────────────────────

export interface Integration {
  id: string;
  name: string;
  type: string;
  status: string;
  url: string;
  description: string;
  icon: string;
  last_sync: string;
  features: string[];
  created_at: string;
  updated_at: string;
}

export const integrations = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ integrations: Integration[]; total: number; summary: Record<string, unknown> }>(`/api/v1/integrations${qs}`);
  },
  get: (id: string) =>
    fetchAPI<{ integration: Integration }>(`/api/v1/integrations/${id}`),
  create: (data: { name: string; type: string; url: string; description?: string }) =>
    fetchAPI<{ integration: Integration; message: string }>('/api/v1/integrations', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Record<string, unknown>) =>
    fetchAPI<{ integration: Integration; message: string }>(`/api/v1/integrations/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/integrations/${id}`, { method: 'DELETE' }),
  test: (id: string) =>
    fetchAPI<{ integration_id: string; status: string; healthy: boolean; latency_ms: number; message: string }>(`/api/v1/integrations/${id}/test`, { method: 'POST' }),
  status: (id: string) =>
    fetchAPI<{ status: { integration_id: string; healthy: boolean; latency_ms: number; last_check: string; details: string } }>(`/api/v1/integrations/${id}/status`),
  sync: (id: string) =>
    fetchAPI<{ integration_id: string; message: string; synced_at: string; items_synced: number }>(`/api/v1/integrations/${id}/sync`, { method: 'POST' }),
};

// ── Automation (Phase 2.3) ───────────────────────────────────

export interface AutomationPipeline {
  id: string;
  project: string;
  branch: string;
  status: string;
  provider: string;
  stages: Array<{ name: string; status: string; duration_ms: number; jobs: number }>;
  duration_ms: number;
  created_at: string;
}

export const automation = {
  pipelines: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ pipelines: AutomationPipeline[]; total: number; summary: Record<string, unknown> }>(`/api/v1/automation/pipelines${qs}`);
  },
  getPipeline: (id: string) =>
    fetchAPI<{ pipeline: AutomationPipeline }>(`/api/v1/automation/pipelines/${id}`),
  recommendations: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ recommendations: Array<Record<string, unknown>>; total: number }>(`/api/v1/automation/recommendations${qs}`);
  },
  templates: () =>
    fetchAPI<{ templates: Array<Record<string, unknown>>; total: number }>('/api/v1/automation/templates'),
  tracking: () =>
    fetchAPI<{ projects: Array<Record<string, unknown>>; total: number; summary: Record<string, unknown> }>('/api/v1/automation/tracking'),
};

// ── Observability (Phase 3.1) ────────────────────────────────

export interface LogEntry {
  timestamp: string;
  level: string;
  service: string;
  pod: string;
  message: string;
  namespace: string;
}

export interface TraceEntry {
  trace_id: string;
  service: string;
  operation: string;
  duration_ms: number;
  spans: number;
  status: string;
  timestamp: string;
}

export interface AlertEntry {
  id: string;
  name: string;
  condition: string;
  severity: string;
  status: string;
  service: string;
  cluster: string;
  fired_at: string;
  description: string;
}

export interface ObservabilitySettings {
  otel_enabled: boolean;
  otel_endpoint: string;
  otel_service_name: string;
  otel_sampling_rate: number;
  otel_insecure: boolean;
  syslog_enabled: boolean;
  syslog_network: string;
  syslog_address: string;
  syslog_tag: string;
  syslog_facility: string;
}

export const observability = {
  overview: () => fetchAPI<Record<string, unknown>>('/api/v1/observability/overview'),
  logs: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ logs: LogEntry[]; total: number }>(`/api/v1/observability/logs${qs}`);
  },
  metrics: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ metrics: Array<Record<string, unknown>>; total: number }>(`/api/v1/observability/metrics${qs}`);
  },
  traces: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ traces: TraceEntry[]; total: number }>(`/api/v1/observability/traces${qs}`);
  },
  dashboards: () => fetchAPI<{ dashboards: Array<Record<string, unknown>>; total: number }>('/api/v1/observability/dashboards'),
  alerts: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ alerts: AlertEntry[]; total: number; summary: Record<string, unknown> }>(`/api/v1/observability/alerts${qs}`);
  },
  resolveAlert: (id: string) => fetchAPI<{ alert: AlertEntry; message: string }>(`/api/v1/observability/alerts/${id}/resolve`, { method: 'POST' }),
  // Observability settings (log export)
  getSettings: () => fetchAPI<ObservabilitySettings>('/api/v1/observability/settings'),
  updateSettings: (data: Partial<ObservabilitySettings>) =>
    fetchAPI<{ message: string }>('/api/v1/observability/settings', { method: 'PUT', body: JSON.stringify(data) }),
  testSyslog: (network: string, address: string) =>
    fetchAPI<{ status: string; message: string }>('/api/v1/observability/settings/test-syslog', { method: 'POST', body: JSON.stringify({ network, address }) }),
  testOTLP: (endpoint: string, insecure: boolean) =>
    fetchAPI<{ status: string; message: string }>('/api/v1/observability/settings/test-otlp', { method: 'POST', body: JSON.stringify({ endpoint, insecure }) }),
};

// ── Cost (Phase 3.3) ─────────────────────────────────────────

export const cost = {
  report: (period?: string) => fetchAPI<{ report: Record<string, unknown>; recommendations: Array<Record<string, unknown>>; total_savings: number }>(`/api/v1/cost/report/${period || 'current_month'}`),
  breakdown: () => fetchAPI<{ breakdown: Array<Record<string, unknown>>; by_cluster: Array<Record<string, unknown>>; by_namespace: Array<Record<string, unknown>> }>('/api/v1/cost/breakdown'),
  recommendations: () => fetchAPI<{ recommendations: Array<Record<string, unknown>>; total: number; total_savings: number }>('/api/v1/cost/recommendations'),
};

// ── Resources (Phase 3.4) ────────────────────────────────────

export const resources = {
  usage: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ resources: Array<Record<string, unknown>>; total: number; summary: Record<string, unknown> }>(`/api/v1/resources/usage${qs}`);
  },
  recommendations: () => fetchAPI<{ recommendations: Array<Record<string, unknown>>; total: number; potential_savings: Record<string, unknown> }>('/api/v1/resources/recommendations'),
};

// ── Backups (Phase 3.5) ──────────────────────────────────────

export const backups = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ backups: Array<Record<string, unknown>>; total: number; summary: Record<string, unknown> }>(`/api/v1/backups${qs}`);
  },
  trigger: (data: { name: string; cluster: string; type?: string }) => fetchAPI<{ backup: Record<string, unknown>; message: string }>('/api/v1/backups', { method: 'POST', body: JSON.stringify(data) }),
  schedules: () => fetchAPI<{ schedules: Array<Record<string, unknown>>; total: number }>('/api/v1/backups/schedules'),
};

// ── AI Extended (Phase 4.1) ───────────────────────────────────

export const aiExtended = {
  analyze: (data: { target: string; context?: string }) => fetchAPI<Record<string, unknown>>('/api/v1/ai/analyze', { method: 'POST', body: JSON.stringify(data) }),
  recommend: (data: { category?: string }) => fetchAPI<{ recommendations: Array<Record<string, unknown>>; total: number }>('/api/v1/ai/recommend', { method: 'POST', body: JSON.stringify(data) }),
  generate: (data: { type: string; name: string; params?: Record<string, string> }) => fetchAPI<{ type: string; name: string; content: string; format: string }>('/api/v1/ai/generate', { method: 'POST', body: JSON.stringify(data) }),
  history: (type?: string) => {
    const qs = type ? `?type=${type}` : '';
    return fetchAPI<{ history: Array<Record<string, unknown>>; total: number }>(`/api/v1/ai/history${qs}`);
  },
  suggestions: () => fetchAPI<{ suggestions: Array<Record<string, unknown>>; total: number }>('/api/v1/ai/suggestions'),
  apply: (data: { type: string; content: string }) => fetchAPI<{ message: string }>('/api/v1/ai/apply', { method: 'POST', body: JSON.stringify(data) }),
};

// ── RAG Knowledge Base ────────────────────────────────────────

export const rag = {
  search: (data: { query: string; top_k?: number; mode?: string; filters?: Record<string, string> }) =>
    fetchAPI<{ results: Array<Record<string, unknown>>; total: number; query: string }>('/api/v1/rag/search', { method: 'POST', body: JSON.stringify(data) }),
  chat: (data: { message: string; top_k?: number; enable_tools?: boolean }) =>
    fetchAPI<{ response: string; sources: Array<Record<string, unknown>>; tokens_used: Record<string, number> }>('/api/v1/rag/chat', { method: 'POST', body: JSON.stringify(data) }),
  chatStream: async (data: { message: string; top_k?: number; enable_tools?: boolean }, onChunk: (chunk: Record<string, unknown>) => void, signal?: AbortSignal): Promise<void> => {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (_bootstrapJwt) headers['Authorization'] = `Bearer ${_bootstrapJwt}`;
    const res = await fetch(`${getBase()}/api/v1/rag/chat/stream`, { method: 'POST', headers, body: JSON.stringify(data), credentials: 'include', signal });
    if (!res.ok) throw new Error(await res.text().catch(() => '') || `HTTP ${res.status}`);
    const reader = res.body?.getReader();
    if (!reader) throw new Error('Streaming not supported');
    const decoder = new TextDecoder();
    let buffer = '';
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        try { onChunk(JSON.parse(line.slice(6))); } catch { /* skip */ }
      }
    }
  },
  documents: (source?: string) => {
    const qs = source ? `?source=${source}` : '';
    return fetchAPI<{ documents: Array<Record<string, unknown>>; total: number }>(`/api/v1/rag/documents${qs}`);
  },
  deleteDocument: (id: string) => fetchAPI<{ message: string }>(`/api/v1/rag/documents/${id}`, { method: 'DELETE' }),
  getDocument: (id: string) => fetchAPI<Record<string, unknown>>(`/api/v1/rag/documents/${id}`),
  updateDocument: (id: string, data: { content: string; metadata?: Record<string, unknown> }) =>
    fetchAPI<{ message: string }>(`/api/v1/rag/documents/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  createDocument: (data: { title: string; source?: string; source_type?: string; content: string; metadata?: Record<string, string> }) =>
    fetchAPI<{ message: string; id: string; title: string }>('/api/v1/rag/documents', { method: 'POST', body: JSON.stringify(data) }),
  stats: () => fetchAPI<{ stats: Record<string, number>; total_documents: number; total_chunks: number; rag_enabled: boolean }>('/api/v1/rag/stats'),
  reindex: () => fetchAPI<{ message: string }>('/api/v1/rag/reindex', { method: 'POST' }),
  ingest: (data: { source: string; source_type?: string; content: string; metadata?: Record<string, string> }) =>
    fetchAPI<{ message: string }>('/api/v1/rag/ingest', { method: 'POST', body: JSON.stringify(data) }),
};

// ── Documentation (Phase 4.2) ─────────────────────────────────

export const docs = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ documents: Array<Record<string, unknown>>; total: number }>(`/api/v1/docs${qs}`);
  },
  create: (data: Record<string, unknown>) => fetchAPI<Record<string, unknown>>('/api/v1/docs', { method: 'POST', body: JSON.stringify(data) }),
  get: (id: string) => fetchAPI<Record<string, unknown>>(`/api/v1/docs/${id}`),
  update: (id: string, data: Record<string, unknown>) => fetchAPI<Record<string, unknown>>(`/api/v1/docs/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) => fetchAPI<Record<string, unknown>>(`/api/v1/docs/${id}`, { method: 'DELETE' }),
  generate: (id: string) => fetchAPI<Record<string, unknown>>(`/api/v1/docs/${id}/generate`, { method: 'POST' }),
  export: (id: string, format?: string) => fetchAPI<Record<string, unknown>>(`/api/v1/docs/${id}/export?format=${format || 'markdown'}`),
  templates: () => fetchAPI<{ templates: Array<Record<string, unknown>>; total: number }>('/api/v1/docs/templates'),
};

// ── Compliance (Phase 4.3) ────────────────────────────────────

export const compliance = {
  reports: () => fetchAPI<{ reports: Array<Record<string, unknown>>; total: number }>('/api/v1/compliance/reports'),
  generate: (framework: string) => fetchAPI<Record<string, unknown>>('/api/v1/compliance/reports', { method: 'POST', body: JSON.stringify({ framework }) }),
  get: (id: string) => fetchAPI<Record<string, unknown>>(`/api/v1/compliance/reports/${id}`),
  exportReport: (id: string, format?: string) => fetchAPI<Record<string, unknown>>(`/api/v1/compliance/reports/${id}/export?format=${format || 'json'}`),
  frameworks: () => fetchAPI<{ frameworks: Array<Record<string, unknown>>; total: number }>('/api/v1/compliance/frameworks'),
  status: () => fetchAPI<Record<string, unknown>>('/api/v1/compliance/status'),
};

// ── Analytics (Phase 4.4) ─────────────────────────────────────

export const analytics = {
  overview: () => fetchAPI<Record<string, unknown>>('/api/v1/analytics/overview'),
  usage: () => fetchAPI<Record<string, unknown>>('/api/v1/analytics/usage'),
  performance: () => fetchAPI<Record<string, unknown>>('/api/v1/analytics/performance'),
  recommendations: () => fetchAPI<{ recommendations: Array<Record<string, unknown>>; total: number }>('/api/v1/analytics/recommendations'),
  trends: () => fetchAPI<Record<string, unknown>>('/api/v1/analytics/trends'),
  insights: () => fetchAPI<{ insights: Array<Record<string, unknown>>; total: number }>('/api/v1/analytics/insights'),
};

// ── Marketplace (Phase 4.5) ───────────────────────────────────

export interface MarketplacePlugin {
  id: string;
  name: string;
  display_name: string;
  description: string;
  version: string;
  type: string;
  category: string;
  author: string;
  license: string;
  installed: boolean;
  running: boolean;
  binary_available: boolean;
  actions: Array<{ name: string; description: string; parameters?: Record<string, unknown> }>;
  config_schema?: Record<string, unknown>;
  requires_config?: string[];
}

export const marketplace = {
  list: () => fetchAPI<{ plugins: MarketplacePlugin[]; total: number }>('/api/v1/marketplace'),
  get: (id: string) => fetchAPI<MarketplacePlugin>(`/api/v1/marketplace/${id}`),
  install: (id: string) =>
    fetchAPI<{ message: string; plugin: PluginInfo }>(`/api/v1/marketplace/${id}/install`, {
      method: 'POST',
    }),
  uninstall: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/marketplace/${id}/uninstall`, {
      method: 'POST',
    }),
};

// ── Knowledge Graph ───────────────────────────────────────────

export const knowledgeGraph = {
  get: () => fetchAPI<{ nodes: Array<Record<string, unknown>>; edges: Array<Record<string, unknown>>; total_nodes: number; total_edges: number }>('/api/v1/graph'),
  relations: (serviceId: string) => fetchAPI<{ service_id: string; relations: Array<Record<string, unknown>>; total: number }>(`/api/v1/graph/services/${serviceId}/relations`),
};

// ── Settings ──────────────────────────────────────────────────

export interface SettingsProfile {
  user_id: string;
  tenant_id: string;
  name: string;
  email: string;
  role: string;
}

export interface SettingsApp {
  dev_mode: boolean;
  log_level: string;
  version: string;
}

export interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  key_suffix: string;
  created_at: string;
  last_used_at: string | null;
}

export const settings = {
  get: () => fetchAPI<{ profile: SettingsProfile; application: SettingsApp }>('/api/v1/settings'),
  updateProfile: (data: { name: string; email: string }) =>
    fetchAPI<{ message: string; profile: { name: string; email: string } }>('/api/v1/settings/profile', { method: 'PUT', body: JSON.stringify(data) }),
  listAPIKeys: () => fetchAPI<{ api_keys: APIKey[]; total: number }>('/api/v1/settings/api-keys'),
  generateAPIKey: (name: string) =>
    fetchAPI<{ api_key: APIKey; full_key: string }>('/api/v1/settings/api-keys', { method: 'POST', body: JSON.stringify({ name }) }),
  revokeAPIKey: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/settings/api-keys/${id}`, { method: 'DELETE' }),
};

// ── Providers (Plugin Engine) ──────────────────────────────

export interface ProviderEntry {
  name: string;
  type: string;
  enabled: boolean;
  actions: string[];
  connection_id?: string;
}

export interface ProviderHealth {
  status: string;
  message?: string;
  latency_ms?: number;
}

export const providers = {
  list: () => fetchAPI<{ providers: ProviderEntry[]; total: number }>('/api/v1/providers'),
  get: (name: string) => fetchAPI<ProviderEntry>(`/api/v1/providers/${name}`),
  health: (name: string) => fetchAPI<ProviderHealth>(`/api/v1/providers/${name}/health`),
  execute: (name: string, action: string, params?: Record<string, unknown>, config?: Record<string, string>) =>
    fetchAPI<{ success: boolean; output: unknown; error?: string }>(`/api/v1/providers/${name}/execute`, {
      method: 'POST',
      body: JSON.stringify({ action, params: params || {}, config: config || {} }),
    }),
  summary: () => fetchAPI<{ providers: Record<string, unknown>[] }>('/api/v1/providers/summary'),
};

// ── Platform Settings ──────────────────────────────────────

export const platformSettings = {
  list: () => fetchAPI<{ settings: Record<string, unknown> }>('/api/v1/settings'),
  get: (key: string) => fetchAPI<{ key: string; value: Record<string, unknown> }>(`/api/v1/settings/${key}`),
  update: (key: string, value: unknown) =>
    fetchAPI<{ key: string; value: unknown; message: string }>(`/api/v1/settings/${key}`, {
      method: 'PUT',
      body: JSON.stringify({ value }),
    }),
  delete: (key: string) =>
    fetchAPI<{ message: string }>(`/api/v1/settings/${key}`, { method: 'DELETE' }),
};

// ── Organization & Workspaces ───────────────────────────────
// Multi-workspace model: one Organization, multiple Workspaces (tenants).

export interface Organization {
  id: string;
  name: string;
  slug: string;
  plan: string;
  created_at: string;
}

export interface Workspace {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  counts?: { teams: number; users: number; environments: number; services: number };
}

export interface WorkspaceMember {
  user_id: string;
  email: string;
  name: string;
  is_super_admin: boolean;
  has_access: boolean;
  roles: string[];
}

export const organization = {
  get: () => fetchAPI<{ organization: Organization; workspace_count: number }>('/api/v1/organization'),
  update: (data: { name: string; slug: string }) =>
    fetchAPI<{ message: string }>('/api/v1/organization', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  setup: (data: { name: string; slug: string }) =>
    fetchAPI<{ message: string; name: string; slug: string }>('/api/v1/setup/organization', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};

export const workspaces = {
  list: () => fetchAPI<{ workspaces: Workspace[]; total: number; current_workspace: string }>('/api/v1/workspaces'),
  get: (id: string) => fetchAPI<{ workspace: Workspace; counts: { services: number; connections: number; teams: number } }>(`/api/v1/workspaces/${id}`),
  create: (data: { name: string; slug: string }) =>
    fetchAPI<{ id: string; name: string; slug: string }>('/api/v1/workspaces', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  update: (id: string, data: { name: string; slug: string }) =>
    fetchAPI<{ message: string }>(`/api/v1/workspaces/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/workspaces/${id}`, { method: 'DELETE' }),
  switch: (id: string) =>
    fetchAPI<{ token: string; workspace_id: string; expires_in: number }>(`/api/v1/workspaces/${id}/switch`, {
      method: 'POST',
    }),
  // Workspace member management (cross-workspace user access)
  listMembers: (id: string) =>
    fetchAPI<{ members: WorkspaceMember[]; total: number }>(`/api/v1/workspaces/${id}/members`),
  addMember: (id: string, userId: string, roleSlug?: string) =>
    fetchAPI<{ message: string }>(`/api/v1/workspaces/${id}/members`, {
      method: 'POST',
      body: JSON.stringify({ user_id: userId, role_slug: roleSlug || 'viewer' }),
    }),
  removeMember: (id: string, userId: string) =>
    fetchAPI<{ message: string }>(`/api/v1/workspaces/${id}/members/${userId}`, { method: 'DELETE' }),
};

// ── Discovery ──────────────────────────────────────────────

export interface DiscoveredService {
  name: string;
  namespace: string;
  cluster: string;
  source: 'pepa' | 'argocd' | 'fluxcd' | 'docker' | 'docker-container' | 'manual';
  status: string;
  health: string;
  replicas: number;
  ready_replicas: number;
  image: string;
  last_updated: string;
  labels: Record<string, string>;
  sync_status: string;
  url?: string;
}

export interface DeploymentInfo {
  name: string;
  namespace: string;
  cluster: string;
  replicas: number;
  ready_replicas: number;
  available_replicas: number;
  image: string;
  images: string[];
  labels: Record<string, string>;
  annotations: Record<string, string>;
  strategy: string;
  created_at: string;
  env: Record<string, string>;
  resource_limits: Record<string, string>;
  resource_requests: Record<string, string>;
}

export interface TeamWorkflowStage {
  key: string;
  label: string;
  color: string;
  auto_promote: boolean;
  requires_approval: boolean;
}

export interface TeamWorkflowConfig {
  id: string;
  team_name: string;
  stages: TeamWorkflowStage[];
  gitops: { provider: string; repo_url: string; branch: string; path: string };
  ci: { provider: string; pipeline: string };
  verification: { checks: string[] };
  created_at: string;
  updated_at: string;
}

export const discovery = {
  services: (params?: Record<string, string>) => {
    const query = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ services: DiscoveredService[]; sources: Record<string, number>; clusters: Record<string, number>; namespaces: Record<string, number>; k8s_clusters: number; k8s_namespaces: number; total: number; total_unfiltered: number }>(`/api/v1/discovery/services${query}`);
  },
  sync: () =>
    fetchAPI<{ message: string; synced: number }>('/api/v1/discovery/sync', { method: 'POST' }),
  sources: () =>
    fetchAPI<{ sources: Array<Record<string, unknown>>; total: number }>('/api/v1/discovery/sources'),
  clusters: () =>
    fetchAPI<{ clusters: Array<Record<string, unknown>>; total: number }>('/api/v1/discovery/clusters'),
  namespaces: () =>
    fetchAPI<{ namespaces: string[]; total: number }>('/api/v1/discovery/namespaces'),
  // FluxCD management
  fluxcdSuspend: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/fluxcd/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/suspend`, { method: 'POST' }),
  fluxcdResume: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/fluxcd/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/resume`, { method: 'POST' }),
  fluxcdReconcile: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/fluxcd/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/reconcile`, { method: 'POST' }),
  fluxcdDelete: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/fluxcd/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  // Kubernetes deployment management
  k8sGet: (cluster: string, namespace: string, name: string) =>
    fetchAPI<DeploymentInfo>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`),
  k8sUpdate: (cluster: string, namespace: string, name: string, body: { image?: string; env?: Record<string, string>; replicas?: number }) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, { method: 'PUT', body: JSON.stringify(body) }),
  k8sScale: (cluster: string, namespace: string, name: string, replicas: number) =>
    fetchAPI<{ message: string; replicas: number }>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/scale`, { method: 'POST', body: JSON.stringify({ replicas }) }),
  k8sRestart: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/restart`, { method: 'POST' }),
  k8sDelete: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ message: string }>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  k8sLogs: (cluster: string, namespace: string, name: string, tail?: number) =>
    fetchAPI<{ logs: string; name: string; namespace: string }>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/logs?tail=${tail || 100}`),
  k8sEvents: (cluster: string, namespace: string, name: string) =>
    fetchAPI<{ events: Array<Record<string, unknown>>; name: string; namespace: string }>(`/api/v1/discovery/k8s/${encodeURIComponent(cluster)}/${encodeURIComponent(namespace)}/${encodeURIComponent(name)}/events`),
  // Docker container logs (for discovered containers from Docker hosts)
  dockerContainerLogs: (host: string, name: string, tail?: number) =>
    fetchAPI<{ logs: string; name: string; cluster: string }>(`/api/v1/discovery/docker-container/${encodeURIComponent(host)}/${encodeURIComponent(name)}/logs?tail=${tail || 200}`),
};

export const teamWorkflows = {
  list: () =>
    fetchAPI<{ workflows: TeamWorkflowConfig[]; total: number }>('/api/v1/team-workflows'),
  get: (team: string) =>
    fetchAPI<TeamWorkflowConfig>(`/api/v1/team-workflows/${team}`),
  save: (team: string, config: Partial<TeamWorkflowConfig>) =>
    fetchAPI<{ message: string; team: string; config: TeamWorkflowConfig }>(`/api/v1/team-workflows/${team}`, { method: 'PUT', body: JSON.stringify(config) }),
  delete: (team: string) =>
    fetchAPI<{ message: string; team: string }>(`/api/v1/team-workflows/${team}`, { method: 'DELETE' }),
};

// ── Docker Hosts ────────────────────────────────────────────

export interface DockerHost {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  host_type: 'local' | 'tcp' | 'ssh';
  host_address: string;
  status: string;
  docker_version: string;
  os_arch: string;
  containers_running: number;
  last_checked_at: string;
  created_at: string;
  updated_at: string;
}

export interface DockerHostTestResult {
  status: string;
  docker_version?: string;
  os_arch?: string;
  containers_running?: number;
  containers_total?: number;
  error?: string;
}

export const dockerHosts = {
  list: () =>
    fetchAPI<{ docker_hosts: DockerHost[]; total: number }>('/api/v1/docker-hosts'),
  get: (id: string) =>
    fetchAPI<DockerHost>(`/api/v1/docker-hosts/${id}`),
  create: (data: { name: string; description?: string; host_type: string; host_address: string; tls_ca_cert?: string; tls_cert?: string; tls_key?: string; ssh_key?: string }) =>
    fetchAPI<DockerHost>('/api/v1/docker-hosts', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<DockerHost>) =>
    fetchAPI<DockerHost>(`/api/v1/docker-hosts/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ status: string }>(`/api/v1/docker-hosts/${id}`, { method: 'DELETE' }),
  test: (id: string) =>
    fetchAPI<DockerHostTestResult>(`/api/v1/docker-hosts/${id}/test`, { method: 'POST' }),
  info: (id: string) =>
    fetchAPI<DockerHost>(`/api/v1/docker-hosts/${id}/info`),
  containers: (id: string, all?: boolean) =>
    fetchAPI<{ containers: DiscoveredDockerContainer[]; total: number; host_id: string; host_name: string }>(`/api/v1/docker-hosts/${id}/containers?all=${all || false}`),
};

// ── Docker Services ─────────────────────────────────────────

export interface DockerContainer {
  name: string;
  image: string;
  state: string;
  status: string;
  ports: string;
  service: string;
}

export interface DiscoveredDockerContainer {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  ports: string;
  labels: Record<string, string>;
  created: string;
  compose_project?: string;
}

export interface DockerService {
  id: string;
  tenant_id: string;
  docker_host_id: string | null; // null = local Docker socket
  name: string;
  compose_yaml: string;
  folder_path: string; // server-side project folder path
  env_vars: Record<string, string>;
  status: string;
  containers: DockerContainer[];
  created_at: string;
  updated_at: string;
}

export const dockerServices = {
  list: () =>
    fetchAPI<{ docker_services: DockerService[]; total: number }>('/api/v1/docker-services'),
  get: (id: string) =>
    fetchAPI<DockerService>(`/api/v1/docker-services/${id}`),
  create: (data: { docker_host_id: string; name: string; compose_yaml?: string; folder_path?: string; env_vars?: Record<string, string> }) =>
    fetchAPI<DockerService>('/api/v1/docker-services', { method: 'POST', body: JSON.stringify(data) }),
  refresh: (id: string) =>
    fetchAPI<DockerService>(`/api/v1/docker-services/${id}/refresh`, { method: 'POST' }),
  restart: (id: string, serviceName?: string) =>
    fetchAPI<{ status: string }>(`/api/v1/docker-services/${id}/restart`, { method: 'POST', body: JSON.stringify({ service_name: serviceName || '' }) }),
  stop: (id: string) =>
    fetchAPI<{ status: string }>(`/api/v1/docker-services/${id}/stop`, { method: 'POST' }),
  start: (id: string) =>
    fetchAPI<{ status: string }>(`/api/v1/docker-services/${id}/start`, { method: 'POST' }),
  delete: (id: string) =>
    fetchAPI<{ status: string }>(`/api/v1/docker-services/${id}`, { method: 'DELETE' }),
  logs: (id: string, service?: string, tail?: number) =>
    fetchAPI<{ logs: string }>(`/api/v1/docker-services/${id}/logs?service=${service || ''}&tail=${tail || 200}`),
  deployLocal: (data: { name: string; compose_yaml?: string; folder_path?: string; env_vars?: Record<string, string> }) =>
    fetchAPI<DockerService>('/api/v1/docker-services/deploy-local', { method: 'POST', body: JSON.stringify(data) }),
};

// ── Helm Repositories ───────────────────────────────────────

export interface HelmRepository {
  id: string;
  tenant_id: string;
  name: string;
  description: string;
  repo_type: 'git' | 'http' | 'oci';
  url: string;
  username: string;
  status: string;
  is_default: boolean;
  last_checked_at: string;
  created_at: string;
  updated_at: string;
}

export const helmRepositories = {
  list: () =>
    fetchAPI<{ helm_repositories: HelmRepository[]; total: number }>('/api/v1/helm-repositories'),
  get: (id: string) =>
    fetchAPI<HelmRepository>(`/api/v1/helm-repositories/${id}`),
  create: (data: { name: string; description?: string; repo_type: string; url: string; username?: string; password?: string; token?: string; ssh_key?: string; ca_cert?: string; is_default?: boolean }) =>
    fetchAPI<HelmRepository>('/api/v1/helm-repositories', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<HelmRepository & { password?: string; token?: string; ssh_key?: string; ca_cert?: string }>) =>
    fetchAPI<HelmRepository>(`/api/v1/helm-repositories/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ status: string }>(`/api/v1/helm-repositories/${id}`, { method: 'DELETE' }),
  // List all charts in a repository
  listCharts: (repoId: string) =>
    fetchAPI<{ charts: HelmChart[]; total: number }>(`/api/v1/helm-repositories/${repoId}/charts`),
  // List all versions for a specific chart
  listChartVersions: (repoId: string, chartName: string) =>
    fetchAPI<{ versions: HelmChartVersion[]; total: number }>(`/api/v1/helm-repositories/${repoId}/charts/${encodeURIComponent(chartName)}/versions`),
  // Download a chart .tgz (returns the download URL)
  downloadChartURL: (repoId: string, chartName: string, version: string) =>
    `/api/v1/helm-repositories/${repoId}/charts/${encodeURIComponent(chartName)}/versions/${encodeURIComponent(version)}/download`,
};

export interface HelmChart {
  name: string;
  description: string;
  latest_version: string;
  app_version: string;
  deprecated: boolean;
  version_count: number;
}

export interface HelmChartVersion {
  version: string;
  app_version: string;
  deprecated: boolean;
  created: string;
  urls: string[];
}

// ── Pipeline Sources ────────────────────────────────────────

export interface PipelineSource {
  id: string;
  tenant_id: string;
  name: string;
  source_type: string;
  description: string;
  connection_id?: string;
  config: Record<string, unknown>;
  parameter_schema?: Record<string, unknown>;
  schema_fetched_at?: string;
  status: string;
  last_error?: string;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface PipelinePreset {
  id: string;
  tenant_id: string;
  source_id: string;
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  created_by?: string;
  use_count: number;
  last_used_at?: string;
  created_at: string;
  updated_at: string;
}

export interface PipelineRun {
  id: string;
  tenant_id: string;
  source_id: string;
  preset_id?: string;
  external_run_id?: string;
  external_url?: string;
  parameters: Record<string, unknown>;
  status: string;
  external_status?: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  logs?: string;
  logs_url?: string;
  triggered_by?: string;
  trigger_type: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface PipelineRunJob {
  id: string;
  run_id: string;
  external_job_id?: string;
  name: string;
  stage?: string;
  status: string;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  log_text?: string;
  log_url?: string;
  runner_name?: string;
  allow_failure: boolean;
  steps?: PipelineJobStep[];
  created_at: string;
}

export interface PipelineJobStep {
  name: string;
  status: string;
  number: number;
  started_at?: string;
  completed_at?: string;
}

export interface EngineStats {
  source_id: string;
  total_runs: number;
  success_count: number;
  failed_count: number;
  running_count: number;
  last_run_at?: string;
  last_run_status?: string;
}

export interface WorkflowGraphStage {
  name: string;
  order: number;
}

export interface WorkflowGraphJob {
  name: string;
  stage: string;
  needs: string[];
  runs_on?: string;
  if?: string;
}

export interface WorkflowGraph {
  name: string;
  source: string;
  stages: WorkflowGraphStage[];
  jobs: WorkflowGraphJob[];
  triggers: string[];
}

export interface TerraformStateResource {
  type: string;
  name: string;
  id: string;
  provider?: string;
  status: string;
}

export interface TerraformState {
  resources: TerraformStateResource[];
  raw_json?: string;
}

export interface TerraformPlan {
  has_changes: boolean;
  add_count: number;
  change_count: number;
  destroy_count: number;
  output_text?: string;
  output_json?: string;
}

export const pipelineSources = {
  list: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ sources: PipelineSource[]; total: number }>(`/api/v1/pipeline-sources${qs}`);
  },
  get: (id: string) =>
    fetchAPI<PipelineSource>(`/api/v1/pipeline-sources/${id}`),
  create: (data: { name: string; source_type: string; description?: string; connection_id?: string; config?: Record<string, unknown> }) =>
    fetchAPI<PipelineSource>('/api/v1/pipeline-sources', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: { name: string; source_type: string; description?: string; connection_id?: string; config?: Record<string, unknown> }) =>
    fetchAPI<PipelineSource>(`/api/v1/pipeline-sources/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ message: string }>(`/api/v1/pipeline-sources/${id}`, { method: 'DELETE' }),
  resolveSchema: (id: string) =>
    fetchAPI<Record<string, unknown>>(`/api/v1/pipeline-sources/${id}/resolve-schema`, { method: 'POST' }),
  state: (id: string) =>
    fetchAPI<TerraformState>(`/api/v1/pipeline-sources/${id}/state`),
  plan: (id: string, params?: Record<string, unknown>) =>
    fetchAPI<TerraformPlan>(`/api/v1/pipeline-sources/${id}/plan`, { method: 'POST', body: params ? JSON.stringify(params) : '{}' }),
  inspect: (id: string) =>
    fetchAPI<Record<string, unknown>>(`/api/v1/pipeline-sources/${id}/inspect`),
  trivyAutoDiscover: () =>
    fetchAPI<{ created: number; existing: number; sources: PipelineSource[] }>('/api/v1/pipeline-sources/trivy/auto-discover', { method: 'POST' }),
  trivyScanAll: () =>
    fetchAPI<{ scanned: number; results: Record<string, unknown>[] }>('/api/v1/pipeline-sources/trivy/scan-all', { method: 'POST' }),
  stats: () =>
    fetchAPI<{ stats: Record<string, EngineStats> }>('/api/v1/pipeline-sources/stats'),
};

export const pipelineRuns = {
  list: (sourceId: string, params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ runs: PipelineRun[]; total: number }>(`/api/v1/pipeline-sources/${sourceId}/runs${qs}`);
  },
  get: (sourceId: string, runId: string) =>
    fetchAPI<{ run: PipelineRun; jobs: PipelineRunJob[] }>(`/api/v1/pipeline-sources/${sourceId}/runs/${runId}`),
  trigger: (sourceId: string, data?: { parameters?: Record<string, unknown>; preset_id?: string }) =>
    fetchAPI<PipelineRun>(`/api/v1/pipeline-sources/${sourceId}/runs`, { method: 'POST', body: JSON.stringify(data || {}) }),
  refresh: (sourceId: string, runId: string) =>
    fetchAPI<PipelineRun>(`/api/v1/pipeline-sources/${sourceId}/runs/${runId}/refresh`, { method: 'POST' }),
  cancel: (sourceId: string, runId: string) =>
    fetchAPI<PipelineRun>(`/api/v1/pipeline-sources/${sourceId}/runs/${runId}/cancel`, { method: 'POST' }),
  sync: (sourceId: string, perPage = 30) =>
    fetchAPI<{ synced: number; skipped: number; total_remote: number }>(`/api/v1/pipeline-sources/${sourceId}/sync-runs?per_page=${perPage}`, { method: 'POST' }),
  jobs: (sourceId: string, runId: string) =>
    fetchAPI<{ jobs: PipelineRunJob[] }>(`/api/v1/pipeline-sources/${sourceId}/runs/${runId}/jobs`),
  logs: (sourceId: string, runId: string, jobId?: string) => {
    const qs = jobId ? `?job_id=${jobId}` : '';
    return fetchAPI<{ logs: string; run_id: string }>(`/api/v1/pipeline-sources/${sourceId}/runs/${runId}/logs${qs}`);
  },
};

export const pipelinePresets = {
  list: (sourceId: string) =>
    fetchAPI<{ presets: PipelinePreset[] }>(`/api/v1/pipeline-sources/${sourceId}/presets`),
  get: (sourceId: string, presetId: string) =>
    fetchAPI<PipelinePreset>(`/api/v1/pipeline-sources/${sourceId}/presets/${presetId}`),
  create: (sourceId: string, data: { name: string; description?: string; parameters?: Record<string, unknown> }) =>
    fetchAPI<PipelinePreset>(`/api/v1/pipeline-sources/${sourceId}/presets`, { method: 'POST', body: JSON.stringify(data) }),
  update: (sourceId: string, presetId: string, data: { name: string; description?: string; parameters?: Record<string, unknown> }) =>
    fetchAPI<PipelinePreset>(`/api/v1/pipeline-sources/${sourceId}/presets/${presetId}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (sourceId: string, presetId: string) =>
    fetchAPI<{ message: string }>(`/api/v1/pipeline-sources/${sourceId}/presets/${presetId}`, { method: 'DELETE' }),
};

export const pipelineWorkflows = {
  graph: (sourceId: string) =>
    fetchAPI<WorkflowGraph>(`/api/v1/pipeline-sources/${sourceId}/workflow-graph`),
};

// ── GitOps ──────────────────────────────────────────────────────────

export interface GitopsRepo {
  id: string;
  tenant_id: string;
  name: string;
  connection_id?: string;
  repo_url: string;
  branch: string;
  path: string;
  engine_type: string;
  scan_status: string;
  scan_error?: string;
  last_scanned_at?: string;
  config: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface GitopsValuesReference {
  kind: string;
  name: string;
  values_key?: string;
  target_path?: string;
  optional?: boolean;
}

export interface GitopsArgoSource {
  repo_url: string;
  path: string;
  target_revision: string;
  helm?: {
    value_files?: string[];
    values?: Record<string, unknown>;
    parameters?: { name: string; value: string }[];
  };
}

export interface GitopsArgoDest {
  server: string;
  namespace: string;
  name?: string;
}

export interface GitopsResource {
  kind: string;
  api_version: string;
  name: string;
  namespace: string;
  file_path: string;
  chart?: string;
  version?: string;
  repo?: string;
  values?: Record<string, unknown>;
  values_from?: GitopsValuesReference[];
  source?: GitopsArgoSource;
  sources?: GitopsArgoSource[];
  dest?: GitopsArgoDest;
  labels?: Record<string, string>;
  depends_on?: string[];
  raw_yaml?: string;
  suspended?: boolean;
  cluster?: string;
  environment?: string;
  layout_role?: string;
  base_path?: string;
  images?: GitopsKustomizeImage[];
  chart_ref?: GitopsFluxChartRef;
}

export interface GitopsKustomizeImage {
  name: string;
  new_name?: string;
  new_tag?: string;
  digest?: string;
}

export interface GitopsFluxChartRef {
  kind: string;
  name: string;
  namespace?: string;
}

export interface GitopsLayout {
  type: string;
  environments?: string[];
  cluster_dirs?: string[];
  base_paths?: string[];
  overlay_paths?: string[];
}

export interface GitopsFileNode {
  name: string;
  path: string;
  type: 'dir' | 'file' | 'environment' | 'cluster' | 'subcluster';
  children?: GitopsFileNode[];
  resources?: GitopsResource[];
  count?: number;
}

export interface GitopsClusterInfo {
  name: string;
  environment: string;
  sub_clusters?: GitopsSubClusterInfo[];
  resource_count: number;
}

export interface GitopsSubClusterInfo {
  name: string;
  resource_count: number;
}

export interface GitopsTopologyNode {
  id: string;
  kind: string;
  name: string;
  namespace: string;
  health: string;
  sync_status: string;
  children: string[];
  parents: string[];
  file_path?: string;
  position_x: number;
  position_y: number;
}

export interface GitopsTopologyEdge {
  from: string;
  to: string;
  type: string;
}

export interface GitopsTopologyGraph {
  nodes: GitopsTopologyNode[];
  edges: GitopsTopologyEdge[];
}

// ── Drift Detection ──────────────────────────────────────────

export interface GitopsDriftEntry {
  kind: string;
  name: string;
  namespace: string;
  cluster?: string;
  drift_type: 'suspended' | 'resumed' | 'version' | 'missing' | 'orphaned' | 'values';
  severity: 'critical' | 'warning' | 'info';
  description: string;
  git_value?: string;
  cluster_value?: string;
  file_path?: string;
  detected_at: string;
}

export interface GitopsDriftSummary {
  total_drifts: number;
  critical: number;
  warning: number;
  info: number;
  suspended: number;
  resumed: number;
  version_drift: number;
  missing: number;
  orphaned: number;
  total_compared: number;
}

export interface GitopsDriftResult {
  repo_id: string;
  repo_name: string;
  cluster_id?: string;
  cluster_name?: string;
  entries: GitopsDriftEntry[];
  summary: GitopsDriftSummary;
  scanned_at: string;
}

// ── Service Blueprints ──────────────────────────────────────────

export interface ServiceBlueprint {
  id: string;
  name: string;
  description: string;
  source_type: 'container' | 'helm_git' | 'helm_http' | 'helm_oci' | 'docker_compose';
  helm_repo_id: string | null;
  image: string;
  chart_url: string;
  chart_name: string;
  chart_version: string;
  chart_path: string;
  namespace: string;
  values_yaml: string;
  cpu: string;
  memory: string;
  replicas: number;
  ports: number[];
  category: string;
  group_ids: string[];
  compose_yaml: string;
  created_at: string;
  // Template metadata (system blueprints)
  slug?: string;
  tenant_id?: string;
  icon?: string;
  language?: string;
  framework?: string;
  tags: string[];
  is_enabled: boolean;
  is_system: boolean;
  dockerfile_tmpl?: string;
  helm_chart?: ServiceTemplateHelmChart;
  cicd_tmpl?: string;
  default_values?: Record<string, unknown>;
  resource_defaults?: Record<string, unknown>;
}

export const blueprints = {
  list: (params?: { type?: string; category?: string; search?: string }) => {
    const qs = new URLSearchParams();
    if (params?.type) qs.set('type', params.type);
    if (params?.category) qs.set('category', params.category);
    if (params?.search) qs.set('search', params.search);
    const query = qs.toString();
    return fetchAPI<{ blueprints: ServiceBlueprint[] }>(`/api/v1/blueprints${query ? `?${query}` : ''}`);
  },
  get: (id: string) =>
    fetchAPI<ServiceBlueprint>(`/api/v1/blueprints/${id}`),
  create: (data: Partial<ServiceBlueprint> & { name: string; source_type: string }) =>
    fetchAPI<ServiceBlueprint>('/api/v1/blueprints', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: Partial<ServiceBlueprint> & { name: string; source_type: string }) =>
    fetchAPI<ServiceBlueprint>(`/api/v1/blueprints/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ ok: boolean }>(`/api/v1/blueprints/${id}`, { method: 'DELETE' }),
  fork: (id: string, data?: { name?: string }) =>
    fetchAPI<ServiceBlueprint>(`/api/v1/blueprints/${id}/fork`, { method: 'POST', body: JSON.stringify(data || {}) }),
  deployDocker: (id: string, dockerHostId: string, envVars?: Record<string, string>) =>
    fetchAPI<DockerService>(`/api/v1/blueprints/${id}/deploy-docker`, { method: 'POST', body: JSON.stringify({ docker_host_id: dockerHostId, env_vars: envVars || {} }) }),
  deployLocal: (id: string, envVars?: Record<string, string>) =>
    fetchAPI<{ status: string; name: string; target: string; containers: unknown[] }>(`/api/v1/blueprints/${id}/deploy-local`, { method: 'POST', body: JSON.stringify({ env_vars: envVars || {} }) }),
};

// ── Blueprint Groups ──────────────────────────────────────────

export interface BlueprintGroup {
  id: string;
  name: string;
  description: string;
  position: number;
  blueprints: ServiceBlueprint[];
  created_at: string;
}

export interface BlueprintDeployResult {
  blueprint_id: string;
  blueprint_name: string;
  status: string;
  deployment_id?: string;
  error?: string;
}

export const blueprintGroups = {
  list: () =>
    fetchAPI<{ groups: BlueprintGroup[] }>('/api/v1/blueprint-groups'),
  create: (data: { name: string; description?: string; position?: number }) =>
    fetchAPI<BlueprintGroup>('/api/v1/blueprint-groups', { method: 'POST', body: JSON.stringify(data) }),
  update: (id: string, data: { name: string; description?: string; position?: number }) =>
    fetchAPI<BlueprintGroup>(`/api/v1/blueprint-groups/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  delete: (id: string) =>
    fetchAPI<{ ok: boolean }>(`/api/v1/blueprint-groups/${id}`, { method: 'DELETE' }),
  reorder: (id: string, blueprintIds: string[]) =>
    fetchAPI<{ ok: boolean }>(`/api/v1/blueprint-groups/${id}/reorder`, { method: 'PUT', body: JSON.stringify({ blueprint_ids: blueprintIds }) }),
  addBlueprints: (id: string, blueprintIds: string[]) =>
    fetchAPI<{ ok: boolean; added: number }>(`/api/v1/blueprint-groups/${id}/blueprints`, { method: 'POST', body: JSON.stringify({ blueprint_ids: blueprintIds }) }),
  removeBlueprint: (id: string, bpId: string) =>
    fetchAPI<{ ok: boolean }>(`/api/v1/blueprint-groups/${id}/blueprints/${bpId}`, { method: 'DELETE' }),
  deployDocker: (id: string, dockerHostId: string, envVars?: Record<string, string>) =>
    fetchAPI<{ results: BlueprintDeployResult[]; total: number }>(`/api/v1/blueprint-groups/${id}/deploy-docker`, { method: 'POST', body: JSON.stringify({ docker_host_id: dockerHostId, env_vars: envVars || {} }) }),
  deployKubernetes: (id: string, clusterId: string, namespace?: string) =>
    fetchAPI<{ results: BlueprintDeployResult[]; total: number }>(`/api/v1/blueprint-groups/${id}/deploy-kubernetes`, { method: 'POST', body: JSON.stringify({ cluster_id: clusterId, namespace: namespace || 'default' }) }),
};

// ── Virtualization (Proxmox) ────────────────────────────────

export interface ProxmoxNode {
  node: string;
  status: string;
  cpu: number;
  maxcpu: number;
  mem: number;
  maxmem: number;
  uptime: number;
  version?: string;
}

export interface ProxmoxVM {
  vmid: number;
  name: string;
  node: string;
  status: string;
  cpu: number;
  maxcpu: number;
  mem: number;
  maxmem: number;
  disk: number;
  maxdisk: number;
  uptime: number;
  template?: number;
  tags?: string;
}

export interface CreateVMRequest {
  node: string;
  vmid?: number;
  name: string;
  template?: number;
  cores?: number;
  memory_mb?: number;
  disk_size?: string;
  storage?: string;
  iso?: string;
  network?: string;
  start?: boolean;
}

export interface CreateContainerRequest {
  node: string;
  vmid?: number;
  hostname: string;
  template: string;
  password?: string;
  cores?: number;
  memory_mb?: number;
  disk_size?: string;
  storage?: string;
  ssh_keys?: string;
  network?: string;
  start?: boolean;
}

export interface ProxmoxStorage {
  storage: string;
  type: string;
  content?: string;
  shared?: number;
  enabled?: number;
  active?: number;
  avail?: number;
  total?: number;
  used?: number;
}

export interface ProxmoxStorageContent {
  volid: string;
  content: string;
  format?: string;
  size?: number;
  ctime?: number;
  notes?: string;
}

export interface ProxmoxOSTemplate {
  template: string;
  package?: string;
  section?: string;
  description?: string;
  os?: string;
  version?: string;
  infopage?: string;
  location?: string;
}

export interface ProxmoxTask {
  upid: string;
  node: string;
  type: string;
  user: string;
  pid?: number;
  pstart?: number;
  starttime: number;
  endtime?: number;
  status?: string;
  exitstatus?: string;
  iserr?: boolean;
}

export interface ProxmoxSyslogLine {
  n: number;
  t: string;
}

export interface DeployDockerRequest {
  node: string;
  vmid?: number;
  hostname: string;
  template: string;
  storage?: string;
  cores?: number;
  memory_mb?: number;
  disk_size?: string;
  network?: string;
  source_type: 'registry' | 'folder' | 'docker_local';
  image?: string;
  folder_path?: string;
  container_name?: string;
  ports?: string[];
  env?: Record<string, string>;
}

export interface DeployDockerResult {
  status?: string;
  vmid?: number;
  ip?: string;
  container_name?: string;
  log?: string;
}

export const virtualization = {
  proxmox: {
    testConnection: () =>
      fetchAPI<{ data: { status?: string; version?: unknown; warning?: string } }>('/api/v1/virtualization/proxmox/test', { method: 'POST' }),
    listNodes: () =>
      fetchAPI<{ data: ProxmoxNode[] }>('/api/v1/virtualization/proxmox/nodes'),
    getNode: (node: string) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/proxmox/nodes/${node}`),
    listVMs: () =>
      fetchAPI<{ data: ProxmoxVM[] }>('/api/v1/virtualization/proxmox/vms'),
    getVM: (node: string, vmid: number) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/proxmox/vms/${node}/${vmid}`),
    createVM: (data: CreateVMRequest) =>
      fetchAPI<{ data: unknown }>('/api/v1/virtualization/proxmox/vms', { method: 'POST', body: JSON.stringify(data) }),
    deleteVM: (node: string, vmid: number) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/proxmox/vms/${node}/${vmid}`, { method: 'DELETE' }),
    vmAction: (node: string, vmid: number, action: 'start' | 'stop' | 'shutdown' | 'reboot') =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/proxmox/vms/${node}/${vmid}/${action}`, { method: 'POST' }),
    listContainers: () =>
      fetchAPI<{ data: ProxmoxVM[] }>('/api/v1/virtualization/proxmox/containers'),
    createContainer: (data: CreateContainerRequest) =>
      fetchAPI<{ data: unknown }>('/api/v1/virtualization/proxmox/containers', { method: 'POST', body: JSON.stringify(data) }),
    deployDocker: (data: DeployDockerRequest) =>
      fetchAPI<{ data: DeployDockerResult }>('/api/v1/virtualization/proxmox/containers/docker', { method: 'POST', body: JSON.stringify(data) }),
    deleteContainer: (node: string, vmid: number) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/proxmox/containers/${node}/${vmid}`, { method: 'DELETE' }),
    containerAction: (node: string, vmid: number, action: 'start' | 'stop') =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/proxmox/containers/${node}/${vmid}/${action}`, { method: 'POST' }),
    clusterResources: () =>
      fetchAPI<{ data: unknown[] }>('/api/v1/virtualization/proxmox/resources'),
    listPools: () =>
      fetchAPI<{ data: unknown[] }>('/api/v1/virtualization/proxmox/pools'),
    listStorage: () =>
      fetchAPI<{ data: ProxmoxStorage[] }>('/api/v1/virtualization/proxmox/storage'),
    listStorageContent: (storage: string, content?: string) =>
      fetchAPI<{ data: ProxmoxStorageContent[] | null }>(`/api/v1/virtualization/proxmox/storage/${encodeURIComponent(storage)}/content${content ? `?content=${encodeURIComponent(content)}` : ''}`),
    getPermissions: () =>
      fetchAPI<{ data: Record<string, Record<string, number>> }>('/api/v1/virtualization/proxmox/permissions'),
    getConnectionInfo: () =>
      fetchAPI<{ data: { url?: string } }>('/api/v1/virtualization/proxmox/connection-info'),
    nextId: () =>
      fetchAPI<{ data: { vmid: number } }>('/api/v1/virtualization/proxmox/next-id'),
    listOSTemplates: (node: string) =>
      fetchAPI<{ data: ProxmoxOSTemplate[] | null }>(`/api/v1/virtualization/proxmox/nodes/${encodeURIComponent(node)}/templates`),
    nodeSyslog: (node: string, limit = 100) =>
      fetchAPI<{ data: ProxmoxSyslogLine[] | null }>(`/api/v1/virtualization/proxmox/nodes/${encodeURIComponent(node)}/syslog?limit=${limit}`),
    nodeTasks: (node: string, limit = 50) =>
      fetchAPI<{ data: ProxmoxTask[] | null }>(`/api/v1/virtualization/proxmox/nodes/${encodeURIComponent(node)}/tasks?limit=${limit}`),
    taskLog: (node: string, upid: string) =>
      fetchAPI<{ data: ProxmoxSyslogLine[] | null }>(`/api/v1/virtualization/proxmox/nodes/${encodeURIComponent(node)}/tasks/${encodeURIComponent(upid)}/log`),
  },
  vmware: {
    testConnection: () =>
      fetchAPI<{ data: { status?: string; version?: unknown } }>('/api/v1/virtualization/vmware/test', { method: 'POST' }),
    listDatacenters: () =>
      fetchAPI<{ data: VMwareDatacenter[] }>('/api/v1/virtualization/vmware/datacenters'),
    listClusters: () =>
      fetchAPI<{ data: VMwareCluster[] }>('/api/v1/virtualization/vmware/clusters'),
    listHosts: () =>
      fetchAPI<{ data: VMwareHost[] }>('/api/v1/virtualization/vmware/hosts'),
    listVMs: () =>
      fetchAPI<{ data: VMwareVM[] }>('/api/v1/virtualization/vmware/vms'),
    getVM: (id: string) =>
      fetchAPI<{ data: VMwareVMDetail }>(`/api/v1/virtualization/vmware/vms/${id}`),
    createVM: (data: VMwareCreateVMRequest) =>
      fetchAPI<{ data: unknown }>('/api/v1/virtualization/vmware/vms', { method: 'POST', body: JSON.stringify(data) }),
    deleteVM: (id: string) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}`, { method: 'DELETE' }),
    vmAction: (id: string, action: 'start' | 'stop' | 'shutdown' | 'reboot' | 'suspend') =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}/${action}`, { method: 'POST' }),
    listDatastores: () =>
      fetchAPI<{ data: VMwareDatastore[] }>('/api/v1/virtualization/vmware/datastores'),
    listNetworks: () =>
      fetchAPI<{ data: VMwareNetwork[] }>('/api/v1/virtualization/vmware/networks'),
    listResourcePools: () =>
      fetchAPI<{ data: VMwareResourcePool[] }>('/api/v1/virtualization/vmware/resource-pools'),
    listSnapshots: (id: string) =>
      fetchAPI<{ data: VMwareSnapshot[] }>(`/api/v1/virtualization/vmware/vms/${id}/snapshots`),
    createSnapshot: (id: string, name: string, description?: string) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}/snapshots`, { method: 'POST', body: JSON.stringify({ name, description }) }),
    deleteSnapshot: (id: string, snapId: string) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}/snapshots/${snapId}`, { method: 'DELETE' }),
    revertSnapshot: (id: string, snapId: string) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}/snapshots/${snapId}/revert`, { method: 'POST' }),
    cloneVM: (id: string, data: { name: string; host?: string; datastore?: string; resource_pool?: string; power_on?: boolean }) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}/clone`, { method: 'POST', body: JSON.stringify(data) }),
    reconfigureVM: (id: string, data: { cores?: number; memory_mib?: number }) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
    migrateVM: (id: string, targetHost: string) =>
      fetchAPI<{ data: unknown }>(`/api/v1/virtualization/vmware/vms/${id}/migrate`, { method: 'POST', body: JSON.stringify({ target_host: targetHost }) }),
    getConnectionInfo: () =>
      fetchAPI<{ data: { url?: string } }>('/api/v1/virtualization/vmware/connection-info'),
  },
};

// ── Virtualization (VMware vCenter) ──────────────────────────

export interface VMwareVM {
  vm: string;
  name: string;
  power_state: string;
  cpu_count: number;
  memory_size_mib: number;
  host?: string;
  cluster?: string;
  guest_OS?: string;
  ip_address?: string;
  guest_host_name?: string;
}

export interface VMwareVMDetail {
  name: string;
  power_state: string;
  cpu: { count: number; cores_per_socket: number };
  memory: { size_MiB: number };
  guest: { os: string; name: string; ip_address: string; host_name: string };
  host: string;
  cluster: string;
}

export interface VMwareHost {
  host: string;
  name: string;
  connection_state: string;
  hardware?: {
    cpu_cores: number;
    memory_size_mib: number;
  };
  memory_usage_mib?: number;
  memory_utilization?: number;
}

export interface VMwareDatacenter {
  datacenter: string;
  name: string;
}

export interface VMwareCluster {
  cluster: string;
  name: string;
}

export interface VMwareDatastore {
  datastore: string;
  name: string;
  type: string;
  free_space: number;
  capacity: number;
}

export interface VMwareNetwork {
  network: string;
  name: string;
  type: string;
}

export interface VMwareResourcePool {
  resource_pool: string;
  name: string;
}

export interface VMwareSnapshot {
  snapshot: string;
  display_name: string;
  description?: string;
  create_time?: string;
  state?: string;
}

export interface VMwareCreateVMRequest {
  name: string;
  host?: string;
  cluster?: string;
  datastore?: string;
  resource_pool?: string;
  cores?: number;
  memory_mib?: number;
  guest_os?: string;
  network?: string;
}

// ── S3 Browser ────────────────────────────────────────────────
export interface S3Bucket {
  name: string;
  creation_date: string;
}

export interface S3Object {
  key: string;
  size: number;
  content_type: string;
  last_modified: string;
}

export interface S3ObjectMeta {
  key: string;
  size: number;
  content_type: string;
  last_modified: string;
  presigned_url: string;
  bucket: string;
}

export interface S3CredentialStatus {
  source: 'user' | 'admin';
  username: string;
  endpoint: string;
}

// encodeKeyPath encodes an S3 object key for use in URL path segments.
// Each path segment is encoded separately, preserving '/' as path separators.
function encodeKeyPath(key: string): string {
  return key.split('/').map(encodeURIComponent).join('/');
}

export const s3Browser = {
  listBuckets: (connectionId: string) =>
    fetchAPI<{ buckets: S3Bucket[]; total: number }>(`/api/v1/s3-browser/${connectionId}/buckets`),

  listObjects: (connectionId: string, bucket: string, prefix?: string) => {
    const qs = prefix ? `?prefix=${encodeURIComponent(prefix)}` : '';
    return fetchAPI<{ objects: S3Object[]; folders: { name: string }[]; total: number; prefix: string }>(
      `/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/objects${qs}`
    );
  },

  getObjectMeta: (connectionId: string, bucket: string, key: string) =>
    fetchAPI<S3ObjectMeta>(
      `/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/objects/${encodeKeyPath(key)}`
    ),

  uploadFile: async (connectionId: string, bucket: string, file: File, key: string) => {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('key', key);
    const res = await fetch(
      `${getBase()}/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/upload`,
      { method: 'POST', body: formData, credentials: 'include' }
    );
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'upload failed' }));
      throw new Error(err.error || 'upload failed');
    }
    return res.json();
  },

  uploadFiles: async (connectionId: string, bucket: string, files: File[], prefix: string, relativePaths?: string[]) => {
    const formData = new FormData();
    formData.append('prefix', prefix);
    for (let i = 0; i < files.length; i++) {
      formData.append('files', files[i]);
      if (relativePaths && relativePaths[i]) {
        formData.append('relative_path', relativePaths[i]);
      }
    }
    const res = await fetch(
      `${getBase()}/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/upload-multiple`,
      { method: 'POST', body: formData, credentials: 'include' }
    );
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'upload failed' }));
      throw new Error(err.error || 'upload failed');
    }
    return res.json() as Promise<{
      message: string;
      bucket: string;
      total: number;
      uploaded: number;
      failed: number;
      results: { key: string; size: number; ok: boolean; error?: string }[];
    }>;
  },

  deleteObject: (connectionId: string, bucket: string, key: string) =>
    fetchAPI<{ message: string }>(
      `/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/objects/${encodeKeyPath(key)}`,
      { method: 'DELETE' }
    ),

  createBucket: (connectionId: string, bucket: string) =>
    fetchAPI<{ message: string; bucket: string }>(
      `/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/create`,
      { method: 'POST' }
    ),

  deleteBucket: (connectionId: string, bucket: string) =>
    fetchAPI<{ message: string; bucket: string }>(
      `/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}`,
      { method: 'DELETE' }
    ),

  getDownloadUrl: (connectionId: string, bucket: string, key: string) =>
    `${getBase()}/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/objects/${encodeKeyPath(key)}?download=true`,

  getPreviewUrl: (connectionId: string, bucket: string, key: string) =>
    `${getBase()}/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/objects/${encodeKeyPath(key)}?preview=true`,

  previewFile: async (connectionId: string, bucket: string, key: string): Promise<{ content: string; contentType: string }> => {
    const url = `${getBase()}/api/v1/s3-browser/${connectionId}/buckets/${encodeURIComponent(bucket)}/objects/${encodeKeyPath(key)}?preview=true`;
    const res = await fetch(url, { credentials: 'include', headers: authHeaders() });
    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'preview failed' }));
      throw new Error(err.error || 'preview failed');
    }
    const contentType = res.headers.get('content-type') || 'application/octet-stream';
    const content = await res.text();
    return { content, contentType };
  },

  getCredentialStatus: (connectionId: string) =>
    fetchAPI<S3CredentialStatus>(`/api/v1/s3-browser/${connectionId}/credential-status`),
};

// ── Plugin Activity ──────────────────────────────────────────

export interface SSHCommandEntry {
  id: string;
  tenant_id: string;
  user_id?: string;
  host_id: string;
  host_name: string;
  username: string;
  command: string;
  exit_code?: number;
  created_at: string;
}

export interface PluginActionEntry {
  id: string;
  tenant_id: string;
  user_id?: string;
  plugin_name: string;
  action: string;
  entity_type: string;
  entity_id: string;
  entity_name: string;
  params?: string;
  status: string;
  error_message?: string;
  ip_address?: string;
  created_at: string;
}

export const pluginActivity = {
  listSSHCommands: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ items: SSHCommandEntry[]; total: number }>(`/api/v1/plugin-activity/ssh-commands${qs}`);
  },
  listPluginActions: (params?: Record<string, string>) => {
    const qs = params ? '?' + new URLSearchParams(params).toString() : '';
    return fetchAPI<{ items: PluginActionEntry[]; total: number }>(`/api/v1/plugin-activity/plugin-actions${qs}`);
  },
};

// ── Security Scanning ──────────────────────────────────────────

export type ScannerType = 'trivy' | 'sonarqube' | 'both';
export type TargetType = 'image' | 'git_repo' | 'filesystem' | 'container' | 'service' | 'sonarqube_project';
export type ScanStatus = 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
export type TriggerType = 'manual' | 'schedule' | 'pipeline' | 'webhook';

export interface ScanTarget {
  id: string;
  tenant_id: string;
  name: string;
  scanner_type: ScannerType;
  target_type: TargetType;
  target_ref: string;
  connection_id?: string;
  scan_config: Record<string, unknown>;
  enabled: boolean;
  last_scan_at?: string;
  last_scan_status?: string;
  last_scan_summary?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface ScanRun {
  id: string;
  tenant_id: string;
  target_id: string;
  scanner_type: ScannerType;
  status: ScanStatus;
  trigger_type: TriggerType;
  started_at?: string;
  completed_at?: string;
  duration_ms?: number;
  result_summary?: Record<string, unknown>;
  result_full?: Record<string, unknown>;
  error_message?: string;
  report_url?: string;
  created_at: string;
  target_name?: string;
  target_ref?: string;
}

export interface ScanSchedule {
  id: string;
  tenant_id: string;
  target_id: string;
  cron_expression: string;
  enabled: boolean;
  last_run_at?: string;
  next_run_at?: string;
  workflow_id?: string;
  created_at: string;
  updated_at: string;
  target_name?: string;
  target_ref?: string;
  scanner_type?: string;
}

export interface SecurityDashboard {
  targets: { total: number; enabled: number };
  recent_scans: ScanRun[];
  scan_summary: { completed: number; failed: number; running: number; pending: number };
  schedules: { total: number; enabled: number };
}

export const securityScan = {
  // Scan Targets
  listTargets: () => fetchAPI<ScanTarget[]>('/api/v1/security/targets'),
  
  getTarget: (id: string) => fetchAPI<ScanTarget>(`/api/v1/security/targets/${id}`),
  
  createTarget: (data: Partial<ScanTarget>) =>
    fetchAPI<ScanTarget>('/api/v1/security/targets', { method: 'POST', body: JSON.stringify(data) }),
  
  updateTarget: (id: string, data: Partial<ScanTarget>) =>
    fetchAPI<ScanTarget>(`/api/v1/security/targets/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  
  deleteTarget: (id: string) =>
    fetchAPI<void>(`/api/v1/security/targets/${id}`, { method: 'DELETE' }),
  
  triggerScan: (id: string) =>
    fetchAPI<{ message: string; target_id: string }>(`/api/v1/security/targets/${id}/scan`, { method: 'POST' }),
  
  // Scan Runs
  listScans: (params?: { target_id?: string; status?: string; limit?: number; offset?: number }) => {
    const qs = params ? '?' + new URLSearchParams(
      Object.entries(params).filter(([_, v]) => v !== undefined).reduce((acc, [k, v]) => ({ ...acc, [k]: String(v) }), {})
    ).toString() : '';
    return fetchAPI<ScanRun[]>(`/api/v1/security/scans${qs}`);
  },
  
  getScan: (id: string) => fetchAPI<ScanRun>(`/api/v1/security/scans/${id}`),
  
  // Scan Schedules
  listSchedules: () => fetchAPI<ScanSchedule[]>('/api/v1/security/schedules'),
  
  getSchedule: (id: string) => fetchAPI<ScanSchedule>(`/api/v1/security/schedules/${id}`),
  
  createSchedule: (data: { target_id: string; cron_expression: string; enabled?: boolean }) =>
    fetchAPI<ScanSchedule>('/api/v1/security/schedules', { method: 'POST', body: JSON.stringify(data) }),
  
  updateSchedule: (id: string, data: { cron_expression?: string; enabled?: boolean }) =>
    fetchAPI<ScanSchedule>(`/api/v1/security/schedules/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  
  deleteSchedule: (id: string) =>
    fetchAPI<void>(`/api/v1/security/schedules/${id}`, { method: 'DELETE' }),
  
  // Dashboard & Bulk Operations
  getDashboard: () => fetchAPI<SecurityDashboard>('/api/v1/security/dashboard-v2'),
  
  scanAll: () => fetchAPI<{ message: string }>('/api/v1/security/scan-all', { method: 'POST' }),
};

