'use client';

import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';
import PluginsClient from './PluginsClient';

export default function PluginsPage() {
  const { isAdmin, hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('plugins', 'read')) {
    return <ForbiddenPage resource="plugins" />;
  }

  return <PluginsClient />;
}
