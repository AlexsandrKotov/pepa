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
  // Sidebar init data — returned by /me in a single request
  enabledPlugins: string[];
  connectionTypes: string[];
  platformName: string;
  getStartedCompleted: boolean;
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
  enabledPlugins: [],
  connectionTypes: [],
  platformName: 'PEPA',
  getStartedCompleted: false,
});

// Module-level cache so subsequent mounts don't re-fetch
let cachedPermissions: string[] | null = null;
let cachedRoles: string[] | null = null;
let cachedEnabledPlugins: string[] | null = null;
let cachedConnectionTypes: string[] | null = null;
let cachedPlatformName = '';
let cachedGetStartedCompleted = false;

// Listen for auth-changed events to reset the module-level cache.
// The PermissionProvider component subscribes to the same event to re-fetch.
if (typeof window !== 'undefined') {
  window.addEventListener('pepa:auth-changed', () => {
    cachedPermissions = null;
    cachedRoles = null;
    cachedEnabledPlugins = null;
    cachedConnectionTypes = null;
    cachedPlatformName = '';
    cachedGetStartedCompleted = false;
  });
}

export function PermissionProvider({ children }: { children: ReactNode }) {
  const [permissions, setPermissions] = useState<string[]>(cachedPermissions || []);
  const [roles, setRoles] = useState<string[]>(cachedRoles || []);
  const [loading, setLoading] = useState(!cachedPermissions);
  const [enabledPlugins, setEnabledPlugins] = useState<string[]>(cachedEnabledPlugins || []);
  const [connectionTypes, setConnectionTypes] = useState<string[]>(cachedConnectionTypes || []);
  const [platformName, setPlatformName] = useState(cachedPlatformName || 'PEPA');
  const [getStartedCompleted, setGetStartedCompleted] = useState(cachedGetStartedCompleted);

  const fetchPermissions = useCallback(() => {
    // If cache is fresh, use it
    if (cachedPermissions) {
      setPermissions(cachedPermissions);
      setRoles(cachedRoles || []);
      setEnabledPlugins(cachedEnabledPlugins || []);
      setConnectionTypes(cachedConnectionTypes || []);
      if (cachedPlatformName) setPlatformName(cachedPlatformName);
      setGetStartedCompleted(cachedGetStartedCompleted);
      setLoading(false);
      return;
    }

    setLoading(true);
    getMe()
      .then((data) => {
        const perms = data.permissions || [];
        cachedPermissions = perms;
        cachedRoles = data.roles || [];
        cachedEnabledPlugins = data.enabled_plugins || [];
        cachedConnectionTypes = data.connection_types || [];
        cachedPlatformName = data.platform_name || '';
        cachedGetStartedCompleted = !!data.get_started_completed;
        setPermissions(perms);
        setRoles(data.roles || []);
        setEnabledPlugins(data.enabled_plugins || []);
        setConnectionTypes(data.connection_types || []);
        if (data.platform_name) setPlatformName(data.platform_name);
        setGetStartedCompleted(!!data.get_started_completed);
      })
      .catch(() => {
        // Don't cache failures — reset to null so next call retries
        cachedPermissions = null;
        cachedRoles = null;
        cachedEnabledPlugins = null;
        cachedConnectionTypes = null;
        cachedPlatformName = '';
        cachedGetStartedCompleted = false;
        setPermissions([]);
        setRoles([]);
        setEnabledPlugins([]);
        setConnectionTypes([]);
        setPlatformName('PEPA');
        setGetStartedCompleted(false);
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
    <PermissionContext.Provider value={{ permissions, roles, loading, hasPermission, hasAnyPermission, isAdmin, enabledPlugins, connectionTypes, platformName, getStartedCompleted }}>
      {children}
    </PermissionContext.Provider>
  );
}

export function usePermission() {
  return useContext(PermissionContext);
}
