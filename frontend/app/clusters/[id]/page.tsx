'use client';

import { Suspense } from 'react';
import ClusterDetailClient from '../ClusterDetailClient';

export default function ClusterDetailPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" /></div>}>
      <ClusterDetailClient />
    </Suspense>
  );
}
