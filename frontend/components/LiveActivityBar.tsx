'use client';

import { useState, useCallback, useEffect, useRef, memo } from 'react';
import { useRouter } from 'next/navigation';
import useSWR from 'swr';
import { getBase } from '@/lib/api';

interface ActiveDeployment {
  id: string;
  service_name: string;
  status: string;
  environment?: string;
  started_at: string;
}

interface LiveActivity {
  type: 'deployment' | 'pipeline' | 'sync';
  label: string;
  detail: string;
  href: string;
  status: 'running' | 'pending' | 'syncing';
}

const MAX_VISIBLE = 3;
const POLL_INTERVAL = 15_000; // 15 seconds

/**
 * Dynamic Island-inspired live activity bar.
 * Shows active operations (deployments, pipelines) as a compact pill
 * that expands on click to show details.
 */
function LiveActivityBarInner() {
  const router = useRouter();
  const [expanded, setExpanded] = useState(false);
  const [activities, setActivities] = useState<LiveActivity[]>([]);
  const prevCountRef = useRef(0);

  // Poll for active deployments
  const { data } = useSWR<{ items?: ActiveDeployment[] }>(
    `${getBase()}/api/v1/deployments?status=running&per_page=5`,
    (url: string) => fetch(url, { headers: { Authorization: `Bearer ${typeof window !== 'undefined' ? localStorage.getItem('pepa-token') || '' : ''}` } }).then(r => r.ok ? r.json() : { items: [] }),
    { refreshInterval: POLL_INTERVAL, revalidateOnFocus: false, dedupingInterval: 10_000 }
  );

  // Map deployments to live activities
  useEffect(() => {
    const deployments = data?.items || [];
    const mapped: LiveActivity[] = deployments.slice(0, MAX_VISIBLE + 2).map(d => ({
      type: 'deployment' as const,
      label: d.service_name || 'Deployment',
      detail: d.environment || d.status,
      href: `/deployments`,
      status: 'running' as const,
    }));
    setActivities(mapped);
  }, [data]);

  // Auto-collapse when no activities
  useEffect(() => {
    if (activities.length === 0 && expanded) {
      setExpanded(false);
    }
  }, [activities.length, expanded]);

  const handleClick = useCallback(() => {
    if (activities.length === 0) return;
    setExpanded(prev => !prev);
  }, [activities.length]);

  const navigateTo = useCallback((href: string) => {
    router.push(href);
    setExpanded(false);
  }, [router]);

  // Don't render if no activities
  if (activities.length === 0) return null;

  const overflow = activities.length > MAX_VISIBLE;
  const visible = activities.slice(0, MAX_VISIBLE);

  return (
    <div
      className="fixed top-12 left-1/2 -translate-x-1/2 z-[90] cursor-pointer select-none"
      onClick={handleClick}
      style={{
        transition: 'all 0.35s var(--ease-ios-spring)',
      }}
    >
      {/* Collapsed pill */}
      {!expanded && (
        <div
          className="flex items-center gap-2 px-3 py-1.5 rounded-full border border-[var(--border)]"
          style={{
            background: 'rgba(255,255,255,0.85)',
            backdropFilter: 'blur(16px) saturate(1.5)',
            WebkitBackdropFilter: 'blur(16px) saturate(1.5)',
            boxShadow: '0 2px 12px rgba(0,0,0,0.08)',
            animation: 'content-reveal 0.3s var(--ease-ios-snappy) forwards',
          }}
        >
          {/* Pulse indicator */}
          <span className="relative flex h-2 w-2">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-[var(--success)] opacity-75" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-[var(--success)]" />
          </span>
          <span className="text-[11px] font-medium text-[var(--text-primary)]">
            {activities.length} active
          </span>
          {overflow && (
            <span className="text-[10px] text-[var(--text-tertiary)]">
              +{activities.length - MAX_VISIBLE}
            </span>
          )}
        </div>
      )}

      {/* Expanded card */}
      {expanded && (
        <div
          className="rounded-2xl border border-[var(--border)] overflow-hidden min-w-[280px] max-w-[360px]"
          style={{
            background: 'rgba(255,255,255,0.9)',
            backdropFilter: 'blur(20px) saturate(1.5)',
            WebkitBackdropFilter: 'blur(20px) saturate(1.5)',
            boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
            animation: 'preview-pop 0.25s var(--ease-ios-spring) forwards',
          }}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-3 py-2 border-b border-[var(--border-light)]">
            <span className="text-[11px] font-semibold text-[var(--text-secondary)] uppercase tracking-wider">
              Live Activities
            </span>
            <span className="text-[10px] text-[var(--text-tertiary)]">
              {activities.length} running
            </span>
          </div>

          {/* Activity items */}
          <div className="py-1">
            {visible.map((activity, idx) => (
              <button
                key={`${activity.type}-${activity.label}-${idx}`}
                className="w-full flex items-center gap-2.5 px-3 py-2 hover:bg-[var(--surface-hover)] transition-colors text-left"
                style={{ transitionTimingFunction: 'var(--ease-ios)' }}
                onClick={(e) => {
                  e.stopPropagation();
                  navigateTo(activity.href);
                }}
              >
                {/* Status dot */}
                <span className="relative flex h-2 w-2 shrink-0">
                  <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-[var(--success)] opacity-75" />
                  <span className="relative inline-flex rounded-full h-2 w-2 bg-[var(--success)]" />
                </span>
                <div className="min-w-0 flex-1">
                  <div className="text-[12px] font-medium text-[var(--text-primary)] truncate">
                    {activity.label}
                  </div>
                  <div className="text-[10px] text-[var(--text-tertiary)] truncate">
                    {activity.detail}
                  </div>
                </div>
                {/* Arrow */}
                <svg className="w-3 h-3 text-[var(--text-tertiary)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                </svg>
              </button>
            ))}
          </div>

          {overflow && (
            <div className="px-3 py-1.5 border-t border-[var(--border-light)]">
              <span className="text-[10px] text-[var(--text-tertiary)]">
                +{activities.length - MAX_VISIBLE} more
              </span>
            </div>
          )}
        </div>
      )}

      {/* Dark mode style */}
      <style>{`
        [data-theme="dark"] [style*="rgba(255,255,255,0.85)"],
        [data-theme="dark"] [style*="rgba(255,255,255,0.9)"] {
          background: rgba(34,38,46,0.88) !important;
          border-color: var(--border) !important;
        }
      `}</style>
    </div>
  );
}

const LiveActivityBar = memo(LiveActivityBarInner);
LiveActivityBar.displayName = 'LiveActivityBar';

export default LiveActivityBar;
