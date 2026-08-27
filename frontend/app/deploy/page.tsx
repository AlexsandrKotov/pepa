'use client';
import { useEffect, Suspense } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';

function DeployRedirect() {
  const router = useRouter();
  const searchParams = useSearchParams();

  useEffect(() => {
    const service = searchParams.get('service');
    const target = service ? `/services?id=${service}` : '/deployments/new';
    router.replace(target);
  }, [searchParams, router]);

  return (
    <div className="flex items-center justify-center min-h-screen bg-[var(--bg)]">
      <div className="loading-spinner" />
    </div>
  );
}

export default function DeployPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center min-h-screen bg-[var(--bg)]"><div className="loading-spinner" /></div>}>
      <DeployRedirect />
    </Suspense>
  );
}
