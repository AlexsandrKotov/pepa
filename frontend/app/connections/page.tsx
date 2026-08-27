'use client';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import ConnectionsClient from './ConnectionsClient';
import ConnectionDetailClient from './[id]/ConnectionDetailClient';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

function ConnectionsPageContent() {
  const searchParams = useSearchParams();
  const connectionId = searchParams.get('id');
  const filterType = searchParams.get('type') || undefined;

  if (connectionId) {
    return <ConnectionDetailClient connectionId={connectionId} />;
  }
  return <ConnectionsClient initialType={filterType} />;
}

export default function ConnectionsPage() {
  const { isAdmin, hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('connections', 'read')) {
    return <ForbiddenPage resource="connections" />;
  }

  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <ConnectionsPageContent />
    </Suspense>
  );
}
