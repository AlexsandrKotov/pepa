'use client';

import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { getMe } from '@/lib/api';

interface PermissionContextValue {
  permissions: string[];
  roles: string[];
  loading: boolean;
  hasPermission: (resource: string, action: string) => boolean;
  hasAnyPermission: (resource: string, actions: string[]) => boolean;
  isAdmin: boolean;
}

const PermissionContext = createContext<PermissionContextValue>({
  permissions: [],
  roles: [],
  loading: true,
  // Default: deny everything until permissions are loaded to prevent
  // admin-only UI elements from flashing for non-admin users.
  hasPermission: () => false,
  hasAnyPermission: () => false,
  isAdmin: false,
});

// Module-level cache so subsequent mounts don't re-fetch
let cachedPermissions: string[] | null = null;
let cachedRoles: string[] | null = null;

// Listen for auth-changed events to reset the module-level cache.
// The PermissionProvider component subscribes to the same event to re-fetch.
if (typeof window !== 'undefined') {
  window.addEventListener('pepa:auth-changed', () => {
    cachedPermissions = null;
    cachedRoles = null;
  });
}

export function PermissionProvider({ children }: { children: ReactNode }) {
  const [permissions, setPermissions] = useState<string[]>(cachedPermissions || []);
  const [roles, setRoles] = useState<string[]>(cachedRoles || []);
  const [loading, setLoading] = useState(!cachedPermissions);

  const fetchPermissions = useCallback(() => {
    // If cache is fresh, use it
    if (cachedPermissions) {
      setPermissions(cachedPermissions);
      setRoles(cachedRoles || []);
      setLoading(false);
      return;
    }

    setLoading(true);
    getMe()
      .then((data) => {
        const perms = data.permissions || [];
        cachedPermissions = perms;
        cachedRoles = data.roles || [];
        setPermissions(perms);
        setRoles(data.roles || []);
      })
      .catch(() => {
        // Don't cache failures — reset to null so next call retries
        cachedPermissions = null;
        cachedRoles = null;
        setPermissions([]);
        setRoles([]);
      })
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    fetchPermissions();
  }, [fetchPermissions]);

  // Re-fetch permissions when auth state changes (e.g. login/logout)
  useEffect(() => {
    const handleAuthChanged = () => {
      // Cache was already reset by the module-level listener below
      fetchPermissions();
    };
    window.addEventListener('pepa:auth-changed', handleAuthChanged);
    return () => window.removeEventListener('pepa:auth-changed', handleAuthChanged);
  }, [fetchPermissions]);

  const isAdmin = roles.some((r) => r === 'admin' || r === 'super_admin');

  const hasPermission = useCallback(
    (resource: string, action: string) => {
      // While loading, deny by default to prevent permission flash.
      if (loading) return false;
      // Admin role always has full access
      if (isAdmin) return true;
      // Wildcard: dev mode returns *:*
      if (permissions.includes('*:*')) return true;
      return permissions.includes(`${resource}:${action}`);
    },
    [permissions, loading, isAdmin],
  );

  const hasAnyPermission = useCallback(
    (resource: string, actions: string[]) => {
      if (loading) return false;
      if (isAdmin) return true;
      if (permissions.includes('*:*')) return true;
      return actions.some((action) => permissions.includes(`${resource}:${action}`));
    },
    [permissions, loading, isAdmin],
  );

  return (
    <PermissionContext.Provider value={{ permissions, roles, loading, hasPermission, hasAnyPermission, isAdmin }}>
      {children}
    </PermissionContext.Provider>
  );
}

export function usePermission() {
  return useContext(PermissionContext);
}
