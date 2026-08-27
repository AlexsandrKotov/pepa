'use client';
import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function PluginsSettingsPage() {
  const router = useRouter();
  useEffect(() => { router.replace('/plugins'); }, [router]);
  return null;
}
