'use client';

import { createContext, useContext, useState, useEffect, useRef, useCallback, type ReactNode } from 'react';
import { getMe } from '@/lib/api';

interface SessionData {
  permissions: string[];
  roles: string[];
  enabledPlugins: string[];
  connectionTypes: string[];
  platformName: string;
  getStartedCompleted: boolean;
}

interface PermissionContextValue extends SessionData {
  loading: boolean;
  hasPermission: (resource: string, action: string) => boolean;
  hasAnyPermission: (resource: string, actions: string[]) => boolean;
  isAdmin: boolean;
}

const defaultSession: SessionData = {
  permissions: [],
  roles: [],
  enabledPlugins: [],
  connectionTypes: [],
  platformName: 'PEPA',
  getStartedCompleted: false,
};

const PermissionContext = createContext<PermissionContextValue>({
  ...defaultSession,
  loading: true,
  // Default: deny everything until permissions are loaded to prevent
  // admin-only UI elements from flashing for non-admin users.
  hasPermission: () => false,
  hasAnyPermission: () => false,
  isAdmin: false,
});

export function PermissionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<SessionData>(defaultSession);
  const [loading, setLoading] = useState(true);
  // Identifies the latest fetch so a slow retry can never overwrite results
  // (or an empty fallback) from a newer request.
  const fetchGen = useRef(0);

  const fetchSession = useCallback((background = false) => {
    const gen = ++fetchGen.current;
    if (!background) setLoading(true);

    // Only "no session" (401) may reset permissions. A 5xx / 429 / network
    // failure (API restart, proxy hiccup, nginx rate limit on /auth/me) is
    // transient: wiping the session there made every gated page render
    // "Access Denied" — even for admins on the Marketplace.
    const MAX_ATTEMPTS = 3;
    const isTransient = (err: unknown) => {
      const status = (err as { status?: number })?.status;
      return status === undefined || status === 429 || status >= 500;
    };

    const apply = (data: Awaited<ReturnType<typeof getMe>>) => {
      if (gen !== fetchGen.current) return;
      setSession({
        permissions: data.permissions || [],
        roles: data.roles || [],
        enabledPlugins: data.enabled_plugins || [],
        connectionTypes: data.connection_types || [],
        platformName: data.platform_name || 'PEPA',
        getStartedCompleted: !!data.get_started_completed,
      });
      setLoading(false);
    };

    const fail = () => {
      if (gen !== fetchGen.current) return;
      if (!background) {
        // Nothing loaded yet (or an explicit re-auth): only now may the
        // session legitimately fall back to "no permissions".
        setSession(defaultSession);
      }
      setLoading(false);
    };

    const attempt = (tryNo: number) => {
      getMe()
        .then(apply)
        .catch((err: unknown) => {
          if (isTransient(err) && tryNo < MAX_ATTEMPTS) {
            setTimeout(() => attempt(tryNo + 1), tryNo * 1000);
            return;
          }
          fail();
        });
    };

    attempt(1);
  }, []);

  // Fetch on mount
  useEffect(() => {
    fetchSession();
  }, [fetchSession]);

  // Re-fetch on auth change (login / logout) — full loading cycle
  // Re-fetch on plugin change — background refresh (no loading flash)
  useEffect(() => {
    const authHandler = () => fetchSession();
    const pluginHandler = () => fetchSession(true);
    window.addEventListener('pepa:auth-changed', authHandler);
    window.addEventListener('pepa:plugins-changed', pluginHandler);
    return () => {
      window.removeEventListener('pepa:auth-changed', authHandler);
      window.removeEventListener('pepa:plugins-changed', pluginHandler);
    };
  }, [fetchSession]);

  const isAdmin = session.roles.some((r) => {
    const lower = r.toLowerCase();
    return lower === 'admin' || lower === 'super_admin' || lower === 'platform_admin' || lower === 'platform admin';
  });

  const hasPermission = useCallback(
    (resource: string, action: string) => {
      if (loading) return false;
      if (isAdmin) return true;
      if (session.permissions.includes('*:*')) return true;
      return session.permissions.includes(`${resource}:${action}`);
    },
    [session.permissions, loading, isAdmin],
  );

  const hasAnyPermission = useCallback(
    (resource: string, actions: string[]) => {
      if (loading) return false;
      if (isAdmin) return true;
      if (session.permissions.includes('*:*')) return true;
      return actions.some((a) => session.permissions.includes(`${resource}:${a}`));
    },
    [session.permissions, loading, isAdmin],
  );

  return (
    <PermissionContext.Provider
      value={{ ...session, loading, hasPermission, hasAnyPermission, isAdmin }}
    >
      {children}
    </PermissionContext.Provider>
  );
}

export function usePermission() {
  return useContext(PermissionContext);
}
