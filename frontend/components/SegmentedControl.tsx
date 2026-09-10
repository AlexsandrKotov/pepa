'use client';

import { useRef, useEffect, useState, useCallback, memo } from 'react';

interface Segment {
  key: string;
  label: string;
  disabled?: boolean;
}

interface SegmentedControlProps {
  segments: Segment[];
  activeKey: string;
  onChange: (key: string) => void;
  size?: 'sm' | 'md';
  className?: string;
}

const sizeClasses = {
  sm: { container: 'h-7 text-[11px]', segment: 'px-2.5' },
  md: { container: 'h-8 text-[12px]', segment: 'px-3' },
};

/**
 * iOS UISegmentedControl — horizontal segments with a sliding indicator.
 * Uses spring animation for the indicator transition.
 * ARIA: role="tablist" with role="tab" on each segment.
 */
function SegmentedControlInner({
  segments,
  activeKey,
  onChange,
  size = 'md',
  className = '',
}: SegmentedControlProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [indicator, setIndicator] = useState({ left: 0, width: 0 });
  const cfg = sizeClasses[size];

  // Calculate indicator position whenever activeKey changes
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const activeEl = container.querySelector(`[data-segment-key="${activeKey}"]`) as HTMLElement | null;
    if (activeEl) {
      setIndicator({ left: activeEl.offsetLeft, width: activeEl.offsetWidth });
    }
  }, [activeKey, segments]);

  // Recalculate on resize
  useEffect(() => {
    const handleResize = () => {
      const container = containerRef.current;
      if (!container) return;
      const activeEl = container.querySelector(`[data-segment-key="${activeKey}"]`) as HTMLElement | null;
      if (activeEl) {
        setIndicator({ left: activeEl.offsetLeft, width: activeEl.offsetWidth });
      }
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, [activeKey]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent, index: number) => {
    const enabledSegments = segments.filter(s => !s.disabled);
    const currentIdx = enabledSegments.findIndex(s => s.key === activeKey);
    let nextIdx = -1;

    if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
      e.preventDefault();
      nextIdx = (currentIdx + 1) % enabledSegments.length;
    } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
      e.preventDefault();
      nextIdx = (currentIdx - 1 + enabledSegments.length) % enabledSegments.length;
    } else if (e.key === 'Home') {
      e.preventDefault();
      nextIdx = 0;
    } else if (e.key === 'End') {
      e.preventDefault();
      nextIdx = enabledSegments.length - 1;
    }

    if (nextIdx >= 0) {
      onChange(enabledSegments[nextIdx].key);
    }
  }, [segments, activeKey, onChange]);

  return (
    <div
      ref={containerRef}
      role="tablist"
      className={`relative inline-flex items-center rounded-lg bg-[var(--border-light)] p-0.5 ${cfg.container} ${className}`}
    >
      {/* Sliding indicator */}
      <div
        className="absolute top-0.5 bottom-0.5 rounded-md bg-[var(--surface)] shadow-sm border border-[var(--border)]"
        style={{
          left: `${indicator.left}px`,
          width: `${indicator.width}px`,
          transition: 'left 0.25s var(--ease-ios-spring), width 0.2s var(--ease-ios)',
        }}
      />

      {/* Segments */}
      {segments.map((seg, idx) => {
        const isActive = seg.key === activeKey;
        return (
          <button
            key={seg.key}
            data-segment-key={seg.key}
            role="tab"
            aria-selected={isActive}
            aria-disabled={seg.disabled}
            tabIndex={isActive ? 0 : -1}
            disabled={seg.disabled}
            onClick={() => !seg.disabled && onChange(seg.key)}
            onKeyDown={(e) => handleKeyDown(e, idx)}
            className={`
              relative z-10 flex items-center justify-center rounded-md
              ${cfg.segment} font-medium whitespace-nowrap
              transition-colors duration-150
              focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-[var(--accent)]
              ${isActive
                ? 'text-[var(--text-primary)]'
                : seg.disabled
                  ? 'text-[var(--text-tertiary)] cursor-not-allowed'
                  : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
              }
            `}
            style={{ transitionTimingFunction: 'var(--ease-ios)' }}
          >
            {seg.label}
          </button>
        );
      })}
    </div>
  );
}

const SegmentedControl = memo(SegmentedControlInner);
SegmentedControl.displayName = 'SegmentedControl';

export default SegmentedControl;
