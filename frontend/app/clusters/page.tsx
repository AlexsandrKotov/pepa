'use client';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import ClustersClient from './ClustersClient';
import ClusterDetailClient from './ClusterDetailClient';
import { usePermission } from '@/hooks/usePermission';
import { ForbiddenPage } from '@/components/PermissionGuard';

function ClustersPageContent() {
  const searchParams = useSearchParams();
  const clusterId = searchParams.get('id');

  if (clusterId) {
    return <ClusterDetailClient />;
  }
  return <ClustersClient />;
}

export default function ClustersPage() {
  const { isAdmin, hasPermission, loading } = usePermission();

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!isAdmin && !hasPermission('clusters', 'read')) {
    return <ForbiddenPage resource="clusters" />;
  }

  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <ClustersPageContent />
    </Suspense>
  );
}
