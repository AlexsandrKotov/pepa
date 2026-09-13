'use client';

import { Suspense } from 'react';
import ServiceDetailClient from '../ServiceDetailClient';

export default function ServiceDetailPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" /></div>}>
      <ServiceDetailClient />
    </Suspense>
  );
}
