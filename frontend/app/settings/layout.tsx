'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

const tabs = [
  { href: '/settings', label: 'General' },
  { href: '/settings/authentication', label: 'Authentication' },
  { href: '/settings/workspaces', label: 'Workspaces' },
  { href: '/settings/users', label: 'Users' },
  { href: '/settings/teams', label: 'Teams' },
];

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { isAdmin, hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('settings', 'read')) {
    return <ForbiddenPage resource="settings" />;
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="page-title-modern">Settings</h1>
        <p className="page-subtitle-modern">Configure platform, workspaces, and users</p>
      </div>

      <div className="flex gap-1 border-b border-[var(--border)] overflow-x-auto">
        {tabs.map(t => {
          const active = t.href === '/settings'
            ? pathname === '/settings'
            : pathname === t.href || pathname.startsWith(t.href + '/');
          return (
            <Link
              key={t.href}
              href={t.href}
              className={`px-4 py-2 text-[13px] font-medium border-b-2 transition-colors whitespace-nowrap ${
                active
                  ? 'border-[var(--accent)] text-[var(--accent)]'
                  : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
              }`}
            >
              {t.label}
            </Link>
          );
        })}
      </div>

      {children}
    </div>
  );
}
