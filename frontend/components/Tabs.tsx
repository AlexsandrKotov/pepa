'use client';

import { useRef, useEffect, useCallback, type KeyboardEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import BrandIcon from './BrandIcon';

export interface TabItem {
  key: string;
  label: string;
  /** BrandIcon name — uses the existing BrandIcon component */
  icon?: string;
  /** Numeric badge shown to the right of the label */
  badge?: number;
  /** When provided the tab renders as a Next.js Link instead of a button */
  href?: string;
}

export interface TabsProps {
  tabs: TabItem[];
  activeKey: string;
  /** Called when a tab is clicked. Not needed when tabs use `href`. */
  onChange?: (key: string) => void;
  /** 'underline' = highlighted pill with accent underline (default), 'rounded' = rounded pill style, 'pills' = modern segmented control with animated active pill */
  variant?: 'underline' | 'rounded' | 'pills';
  /** 'md' = default page tabs, 'sm' = compact for panels / modals */
  size?: 'sm' | 'md';
  className?: string;
}

/**
 * Unified tab navigation component used across all pages.
 * Supports icons (BrandIcon), numeric badges, two visual variants,
 * keyboard arrow-key navigation, and overflow scrolling.
 * Active tab is highlighted with a translucent background — no JS measurements needed.
 */
export default function Tabs({
  tabs,
  activeKey,
  onChange,
  variant = 'underline',
  size = 'md',
  className = '',
}: TabsProps) {
  const router = useRouter();
  const activeRef = useRef<HTMLButtonElement | HTMLAnchorElement | null>(null);

  // Focus and scroll-into-view for the active tab (keyboard a11y)
  useEffect(() => {
    if (activeRef.current) {
      activeRef.current.scrollIntoView({ behavior: 'smooth', block: 'nearest', inline: 'nearest' });
      activeRef.current.focus();
    }
  }, [activeKey]);

  // Keyboard navigation: ← → arrows
  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLDivElement>) => {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
      e.preventDefault();
      const idx = tabs.findIndex(t => t.key === activeKey);
      if (idx === -1) return;
      const next = e.key === 'ArrowLeft'
        ? (idx - 1 + tabs.length) % tabs.length
        : (idx + 1) % tabs.length;
      const nextTab = tabs[next];
      if (nextTab.href) {
        router.push(nextTab.href);
      } else {
        onChange?.(nextTab.key);
      }
    },
    [tabs, activeKey, onChange, router],
  );

  const isSm = size === 'sm';
  const padX = isSm ? 'px-3' : 'px-5';
  const padY = isSm ? 'py-1.5' : 'py-3';
  const textSize = isSm ? 'text-[12px]' : 'text-[14px]';

  const isPills = variant === 'pills';
  const activeClasses = variant === 'rounded'
    ? 'text-[var(--accent)] bg-[var(--surface)] shadow-sm'
    : variant === 'pills'
      ? 'text-white bg-gradient-to-r from-[var(--accent)] to-[var(--accent)]/85 shadow-md shadow-[var(--accent)]/20'
      : 'text-[var(--accent)] bg-[var(--accent)]/10 border-b-2 border-[var(--accent)]';

  const inactiveClasses = variant === 'rounded'
    ? 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--surface-hover)]'
    : variant === 'pills'
      ? 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--accent)]/5'
      : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--accent)]/5 border-b-2 border-transparent';

  const containerClasses = variant === 'pills'
    ? `inline-flex items-center gap-1 p-1 rounded-xl bg-[var(--surface-hover)]/60 border border-[var(--border)]/60 backdrop-blur-sm ${className}`
    : variant === 'rounded'
      ? `relative flex gap-1 overflow-x-auto scrollbar-hide border-b border-[var(--border)] ${className}`
      : `relative flex gap-1 overflow-x-auto scrollbar-hide`;

  const tabRadius = isPills ? 'rounded-lg' : 'rounded-t-lg';

  return (
    <div
      role="tablist"
      aria-orientation="horizontal"
      onKeyDown={handleKeyDown}
      className={`${containerClasses} ${!isPills && variant !== 'rounded' ? className : ''}`}
    >
      {tabs.map(t => {
        const isActive = t.key === activeKey;
        const baseClass = `${padX} ${padY} ${textSize} font-medium whitespace-nowrap transition-all duration-200 outline-none focus-visible:ring-1 focus-visible:ring-[var(--accent)]/30 ${tabRadius} ${isActive ? activeClasses : inactiveClasses}`;

        const content = (
          <>
            {t.icon && (
              <span className="mr-1.5 inline-flex items-center">
                <BrandIcon name={t.icon} size={14} />
              </span>
            )}
            <span>{t.label}</span>
            {t.badge !== undefined && t.badge > 0 && (
              <span className={`ml-1.5 text-[10px] px-1.5 py-0.5 rounded-full ${
                isPills
                  ? isActive ? 'bg-white/20 text-white' : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'
                  : isActive ? 'bg-[var(--accent)]/15 text-[var(--accent)]' : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'
              }`}>
                {t.badge}
              </span>
            )}
          </>
        );

        if (t.href) {
          return (
            <Link
              key={t.key}
              href={t.href}
              ref={isActive ? (activeRef as React.Ref<HTMLAnchorElement>) : undefined}
              role="tab"
              aria-selected={isActive}
              tabIndex={isActive ? 0 : -1}
              className={`${baseClass} inline-flex items-center ${isActive ? 'font-semibold' : ''}`}
            >
              {content}
            </Link>
          );
        }

        return (
          <button
            key={t.key}
            ref={isActive ? (activeRef as React.Ref<HTMLButtonElement>) : undefined}
            role="tab"
            aria-selected={isActive}
            tabIndex={isActive ? 0 : -1}
            onClick={() => onChange?.(t.key)}
            className={`${baseClass} inline-flex items-center ${isActive ? 'font-semibold' : ''}`}
          >
            {content}
          </button>
        );
      })}
    </div>
  );
}
