'use client';

import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
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

  const fetchSession = useCallback(() => {
    setLoading(true);
    getMe()
      .then((data) => {
        setSession({
          permissions: data.permissions || [],
          roles: data.roles || [],
          enabledPlugins: data.enabled_plugins || [],
          connectionTypes: data.connection_types || [],
          platformName: data.platform_name || 'PEPA',
          getStartedCompleted: !!data.get_started_completed,
        });
      })
      .catch(() => {
        setSession(defaultSession);
      })
      .finally(() => setLoading(false));
  }, []);

  // Fetch on mount
  useEffect(() => {
    fetchSession();
  }, [fetchSession]);

  // Re-fetch on auth change (login / logout)
  useEffect(() => {
    const handler = () => fetchSession();
    window.addEventListener('pepa:auth-changed', handler);
    window.addEventListener('pepa:plugins-changed', handler);
    return () => {
      window.removeEventListener('pepa:auth-changed', handler);
      window.removeEventListener('pepa:plugins-changed', handler);
    };
  }, [fetchSession]);

  const isAdmin = session.roles.some((r) => r === 'admin' || r === 'super_admin');

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
