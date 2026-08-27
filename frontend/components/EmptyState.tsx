import Link from 'next/link';

import type { ReactNode } from 'react';

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description: string;
  actionHref?: string;
  actionLabel?: string;
  actionOnClick?: () => void;
  secondaryHref?: string;
  secondaryLabel?: string;
}

/**
 * Standard empty state shown when a page has no data yet.
 * Answers "what is this and what do I do first" for beginners.
 */
export default function EmptyState({
  icon = '📭',
  title,
  description,
  actionHref,
  actionLabel,
  actionOnClick,
  secondaryHref,
  secondaryLabel,
}: EmptyStateProps) {
  return (
    <div className="card">
      <div className="px-6 py-12 text-center">
        <div className="text-4xl mb-3 opacity-30 flex items-center justify-center">{icon}</div>
        <h3 className="text-[15px] font-medium text-[var(--text-primary)] mb-1">{title}</h3>
        <p className="text-[13px] text-[var(--text-secondary)] max-w-md mx-auto mb-5">{description}</p>
        <div className="flex items-center justify-center gap-3">
          {actionLabel && (actionOnClick ? (
            <button
              onClick={actionOnClick}
              className="px-4 py-2 bg-[var(--accent)] text-white text-[13px] rounded-lg hover:opacity-90 transition-opacity"
            >
              {actionLabel}
            </button>
          ) : actionHref ? (
            <Link
              href={actionHref}
              className="px-4 py-2 bg-[var(--accent)] text-white text-[13px] rounded-lg hover:opacity-90 transition-opacity"
            >
              {actionLabel}
            </Link>
          ) : null)}
          {secondaryHref && secondaryLabel && (
            <Link
              href={secondaryHref}
              className="px-4 py-2 border border-[var(--border)] text-[var(--text-secondary)] text-[13px] rounded-lg hover:bg-[var(--bg)] transition-colors"
            >
              {secondaryLabel}
            </Link>
          )}
        </div>
      </div>
    </div>
  );
}
