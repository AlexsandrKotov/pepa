'use client';

import type { ReactNode } from 'react';
import { DOT_CLASS } from './tones';
import type { Tone } from '@/lib/filter-labels';

export interface ActiveChip {
  id: string;
  /** Field caption rendered before the value, e.g. "Status". */
  field: string;
  label: string;
  tone?: Tone;
  onRemove: () => void;
}

interface FilterChipsProps {
  chips: ActiveChip[];
  onClearAll?: () => void;
  /** "12 of 148 clusters" — the honest result summary. */
  summary?: ReactNode;
  className?: string;
}

/**
 * Removable chips are the visible source of truth for applied filters:
 * whatever produced them (pill, menu, URL, deep link) they read the same way.
 */
export default function FilterChips({ chips, onClearAll, summary, className = '' }: FilterChipsProps) {
  if (chips.length === 0 && !summary) return null;

  return (
    <div className={`filter-chips-row ${className}`}>
      <div className="flex flex-wrap items-center gap-1.5" aria-live="polite">
        {chips.map(chip => (
          <span key={chip.id} className="filter-chip">
            {chip.tone && <span className={`filter-pill-dot ${DOT_CLASS[chip.tone]}`} aria-hidden="true" />}
            <span className="filter-chip-field">{chip.field}</span>
            <span className="filter-chip-value">{chip.label}</span>
            <button
              type="button"
              className="filter-chip-remove"
              onClick={chip.onRemove}
              aria-label={`Remove ${chip.field} filter ${chip.label}`}
            >
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </span>
        ))}
        {chips.length > 0 && onClearAll && (
          <button type="button" className="filter-clear-all" onClick={onClearAll}>
            Clear all
          </button>
        )}
      </div>
      {summary && <div className="filter-summary">{summary}</div>}
    </div>
  );
}
