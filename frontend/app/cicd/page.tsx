'use client';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function CICDPage() {
  const router = useRouter();
  useEffect(() => { router.replace('/pipelines?tab=providers'); }, [router]);
  return null;
}
