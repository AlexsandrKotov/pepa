'use client';

import { useRef, useEffect, useCallback, useState, type KeyboardEvent } from 'react';
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
  /** 'underline' = flat border-b accent (default), 'rounded' = rounded-t pill style */
  variant?: 'underline' | 'rounded';
  /** 'md' = default page tabs, 'sm' = compact for panels / modals */
  size?: 'sm' | 'md';
  className?: string;
}

/**
 * Unified tab navigation component used across all pages.
 * Supports icons (BrandIcon), numeric badges, two visual variants,
 * keyboard arrow-key navigation, animated underline indicator, and overflow scrolling.
 */
export default function Tabs({
  tabs,
  activeKey,
  onChange,
  variant = 'underline',
  size = 'md',
  className = '',
}: TabsProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const activeRef = useRef<HTMLButtonElement | HTMLAnchorElement | null>(null);
  const router = useRouter();
  const [indicator, setIndicator] = useState<{ left: number; width: number }>({ left: 0, width: 0 });

  // Compute indicator position from the active tab element
  useEffect(() => {
    if (activeRef.current && containerRef.current) {
      const containerRect = containerRef.current.getBoundingClientRect();
      const tabRect = activeRef.current.getBoundingClientRect();
      setIndicator({
        left: tabRect.left - containerRect.left + containerRef.current.scrollLeft,
        width: tabRect.width,
      });
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
  const padX = isSm ? 'px-3' : 'px-4';
  const padY = isSm ? 'py-1.5' : 'py-2';
  const textSize = isSm ? 'text-[12px]' : 'text-[13px]';

  const activeClasses = variant === 'rounded'
    ? 'text-[var(--accent)] bg-[var(--surface)]'
    : 'text-[var(--accent)]';

  const inactiveClasses = variant === 'rounded'
    ? 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--surface-hover)]'
    : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]';

  return (
    <div
      ref={containerRef}
      role="tablist"
      aria-orientation="horizontal"
      onKeyDown={handleKeyDown}
      className={`relative flex gap-1 overflow-x-auto scrollbar-hide ${variant === 'rounded' ? 'border-b border-[var(--border)]' : ''} ${className}`}
    >
      {tabs.map(t => {
        const isActive = t.key === activeKey;
        const baseClass = `${padX} ${padY} ${textSize} font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-1 focus-visible:ring-[var(--accent)]/30 rounded-t ${isActive ? activeClasses : inactiveClasses}`;

        const content = (
          <>
            {t.icon && (
              <span className="mr-1.5 inline-flex items-center">
                <BrandIcon name={t.icon} size={14} />
              </span>
            )}
            <span>{t.label}</span>
            {t.badge !== undefined && t.badge > 0 && (
              <span className="ml-1.5 text-[10px] px-1.5 py-0.5 rounded-full bg-[var(--border-light)] text-[var(--text-tertiary)]">
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
            className={`${baseClass} inline-flex items-center`}
          >
            {content}
          </button>
        );
      })}

      {/* Animated underline indicator */}
      <span
        className="absolute bottom-0 h-[2px] bg-[var(--accent)] transition-[left,width] duration-200 ease-out"
        style={{ left: indicator.left, width: indicator.width }}
      />
    </div>
  );
}
