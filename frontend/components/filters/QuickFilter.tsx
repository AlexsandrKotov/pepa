'use client';

import { DOT_CLASS } from './tones';
import type { FilterOption } from './types';
import { valueLabel, valueTone } from '@/lib/filter-labels';

interface QuickFilterProps {
  /** Field key, used for the accessible group name (e.g. "status"). */
  field: string;
  label: string;
  options: FilterOption[];
  /** '' means "no constraint". */
  value: string;
  onChange: (value: string) => void;
  /** Caption of the neutral option. Defaults to "All". */
  allLabel?: string;
  allCount?: number;
  className?: string;
}

/**
 * The filters people actually use are always visible as a pill row —
 * no click-to-open dropdown, no "All Statuses" placeholder state.
 * Zero-count options stay visible but disabled, so an empty result set
 * never looks like a broken control.
 */
export default function QuickFilter({
  field,
  label,
  options,
  value,
  onChange,
  allLabel = 'All',
  allCount,
  className = '',
}: QuickFilterProps) {
  const visible = options.filter(option => !(option.hideWhenZero && (option.count ?? 0) === 0 && option.value !== value));

  return (
    <div className={`filter-pill-group ${className}`} role="group" aria-label={label}>
      <span className="filter-group-label" aria-hidden="true">{label}</span>
      <button
        type="button"
        className="filter-pill"
        data-active={value === '' ? 'true' : 'false'}
        aria-pressed={value === ''}
        onClick={() => value !== '' && onChange('')}
      >
        {allLabel}
        {allCount !== undefined && <span className="filter-pill-count">{allCount}</span>}
      </button>

      {visible.map(option => {
        const active = value === option.value;
        const empty = option.count !== undefined && option.count === 0 && !active;
        const tone = option.tone ?? valueTone(option.value);
        return (
          <button
            key={option.value}
            type="button"
            className="filter-pill"
            data-active={active ? 'true' : 'false'}
            aria-pressed={active}
            disabled={option.disabled || empty}
            title={empty ? `No ${option.label.toLowerCase()} ${field}` : option.label}
            onClick={() => onChange(active ? '' : option.value)}
          >
            <span className={`filter-pill-dot ${DOT_CLASS[tone]}`} aria-hidden="true" />
            {option.label || valueLabel(option.value)}
            {option.count !== undefined && <span className="filter-pill-count">{option.count}</span>}
          </button>
        );
      })}
    </div>
  );
}
