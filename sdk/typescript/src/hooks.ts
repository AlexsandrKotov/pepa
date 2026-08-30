/**
 * React hooks for PEPA plugins
 */

import { useState, useEffect } from 'react';
import { UserContext } from './types';

/**
 * Hook to get current user context
 */
export function useUser(): UserContext | null {
  const [user, setUser] = useState<UserContext | null>(null);
  
  useEffect(() => {
    const userStr = localStorage.getItem('pepa:user');
    if (userStr) {
      setUser(JSON.parse(userStr));
    }
  }, []);
  
  return user;
}

/**
 * Hook to get tenant ID
 */
export function useTenantId(): string {
  const user = useUser();
  return user?.tenantId || '';
}

/**
 * Hook to check if user has permission
 */
export function useHasPermission(permission: string): boolean {
  const user = useUser();
  return user?.permissions?.includes(permission) || false;
}

/**
 * Hook to check if user has role
 */
export function useHasRole(role: string): boolean {
  const user = useUser();
  return user?.roles?.includes(role) || false;
}
