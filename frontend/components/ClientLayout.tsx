'use client';

import { useEffect, useState, useRef } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import dynamic from 'next/dynamic';
import { isAuthenticated, getBootstrapStatus, refreshMe } from '@/lib/api';
import Breadcrumbs from '@/components/Breadcrumbs';
import Sidebar from '@/components/Sidebar';
import TopBar from '@/components/TopBar';
import CommandPalette from '@/components/CommandPalette';

// Lazy-load AI widget — only mounted inside the authenticated shell
const DashboardAIWidget = dynamic(() => import('@/components/DashboardAIWidget'), { ssr: false });

// Module-level cache: once bootstrap check passes, no need to re-check
// for the lifetime of the SPA session.
let bootstrapVerified = false;

// Pages that render without the standard shell (sidebar + topbar)
const SHELL_LESS_PAGES = ['/login'];

export default function ClientLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const [checked, setChecked] = useState(bootstrapVerified);
  const refreshTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isShellLess = SHELL_LESS_PAGES.some(p => pathname === p || pathname.startsWith(p + '/'));

  // ── Automatic session refresh ──────────────────────────────
  // Keep the JWT alive by calling /api/v1/auth/refresh periodically.
  // This prevents the user from being kicked out after the session expires.
  useEffect(() => {
    if (isShellLess) return;

    const doRefresh = async () => {
      if (!isAuthenticated()) return;
      await refreshMe();
    };

    // Refresh on tab/window focus (user came back)
    const onFocus = () => { doRefresh(); };
    window.addEventListener('focus', onFocus);

    // Also refresh every 30 minutes while the tab is active
    refreshTimerRef.current = setInterval(doRefresh, 30 * 60 * 1000);

    return () => {
      window.removeEventListener('focus', onFocus);
      if (refreshTimerRef.current) clearInterval(refreshTimerRef.current);
    };
  }, [isShellLess]);

  useEffect(() => {
    // Skip auth check on shell-less pages (e.g. login)
    if (SHELL_LESS_PAGES.some(p => pathname === p || pathname.startsWith(p + '/'))) {
      setChecked(true);
      return;
    }

    // If already verified in this session, skip the API call entirely
    if (bootstrapVerified) {
      setChecked(true);
      return;
    }

    // Always check bootstrap status first — if first-run setup is pending,
    // redirect to login so the user must enter the bootstrap token.
    getBootstrapStatus()
      .then(({ needed, in_progress }) => {
        if (needed || in_progress) {
          router.push('/login');
          return;
        }
        // Always redirect to login if not authenticated (no dev bypass)
        if (!isAuthenticated()) {
          router.push('/login');
          return;
        }
        // Mark as verified for subsequent navigations
        bootstrapVerified = true;
      })
      .catch(() => {
        // If API is unreachable, redirect to login if not authenticated
        if (!isAuthenticated()) {
          router.push('/login');
        }
      })
      .finally(() => setChecked(true));
    // Only run on mount — pathname/router excluded to avoid re-running on every navigation
    // which can interfere with Next.js client-side router transitions.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Avoid rendering content before auth/bootstrap check completes
  if (!checked && !isShellLess) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-[var(--bg)]">
        <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  // Shell-less pages (login, etc.) — render children directly
  if (isShellLess) {
    return <>{children}</>;
  }

  return (
    <div className="flex h-screen overflow-hidden bg-[var(--bg)]">
      <Sidebar />
      <div className="flex-1 flex flex-col min-w-0 overflow-x-hidden">
        <TopBar />
        <main className="flex-1 overflow-y-auto">
          <div className="px-6 py-6 max-w-[1400px] mx-auto w-full">
            <Breadcrumbs />
            {children}
          </div>
        </main>
      </div>
      <DashboardAIWidget />
      <CommandPalette />
    </div>
  );
}
