'use client';
import { useEffect, Suspense } from 'react';
import { useRouter } from 'next/navigation';

function SelfServiceDeployRedirect() {
  const router = useRouter();

  useEffect(() => {
    router.replace('/services/new');
  }, [router]);

  return (
    <div className="flex items-center justify-center min-h-screen bg-[var(--bg)]">
      <div className="loading-spinner" />
    </div>
  );
}

export default function SelfServiceDeployPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center min-h-screen bg-[var(--bg)]"><div className="loading-spinner" /></div>}>
      <SelfServiceDeployRedirect />
    </Suspense>
  );
}
