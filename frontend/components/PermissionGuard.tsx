'use client';

import { usePermission } from '@/hooks/usePermission';
import { type ReactNode } from 'react';

interface PermissionGuardProps {
  resource: string;
  action?: string;
  children: ReactNode;
  fallback?: ReactNode;
}

export default function PermissionGuard({ resource, action = 'read', children, fallback }: PermissionGuardProps) {
  const { hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!hasPermission(resource, action)) {
    if (fallback) return <>{fallback}</>;
    return <ForbiddenPage resource={resource} />;
  }

  return <>{children}</>;
}

export function ForbiddenPage({ resource }: { resource?: string }) {
  return (
    <div className="-mx-6 -my-6 min-h-full flex items-center justify-center page-mesh-bg">
      <div className="text-center px-6 py-16">
        <div className="inline-flex items-center justify-center w-16 h-16 rounded-2xl bg-red-500/10 border border-red-500/20 mb-6">
          <svg className="w-8 h-8 text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
          </svg>
        </div>
        <h1 className="text-[20px] font-semibold text-[var(--text-primary)] mb-2">Access Denied</h1>
        <p className="text-[13px] text-[var(--text-secondary)] max-w-sm mx-auto">
          {resource
            ? `You don't have permission to access "${resource}". Contact your administrator to request access.`
            : 'You don\'t have permission to access this page. Contact your administrator to request access.'}
        </p>
        <a
          href="/"
          className="inline-flex items-center gap-2 mt-6 px-4 py-2 rounded-lg bg-[var(--surface)] border border-[var(--border)] text-[13px] text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5L3 12m0 0l7.5-7.5M3 12h18" />
          </svg>
          Back to Dashboard
        </a>
      </div>
    </div>
  );
}
