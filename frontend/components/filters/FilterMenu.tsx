'use client';

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { DOT_CLASS } from './tones';
import type { FilterGroup } from './types';

interface FilterMenuProps {
  groups: FilterGroup[];
  /** Selected values per group key. */
  values: Record<string, string[]>;
  onToggle: (groupKey: string, value: string) => void;
  onClearGroup: (groupKey: string) => void;
  triggerLabel?: string;
  /** Filter funnel or sort arrows — matches the intent of the menu. */
  variant?: 'filter' | 'sort';
  align?: 'left' | 'right';
  activeCount?: number;
  /** Keep the panel mounted but inert — used to measure the trigger on first paint. */
  className?: string;
}

interface PanelPosition {
  top: number;
  left: number;
  maxHeight: number;
}

const PANEL_WIDTH = 380;

/**
 * Linear-style filter menu: one click opens grouped facets with a typeahead,
 * multi-select applies instantly, and every choice becomes a removable chip.
 * Custom listbox instead of <select> so options can carry colors, counts and search.
 */
export default function FilterMenu({
  groups,
  values,
  onToggle,
  onClearGroup,
  triggerLabel = 'More filters',
  variant = 'filter',
  align = 'left',
  activeCount = 0,
  className = '',
}: FilterMenuProps) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [activeGroup, setActiveGroup] = useState(groups[0]?.key ?? '');
  const [query, setQuery] = useState('');
  const [focusIdx, setFocusIdx] = useState(0);
  const [pos, setPos] = useState<PanelPosition | null>(null);
  const [mobile, setMobile] = useState(false);

  const currentGroup = useMemo(
    () => groups.find(group => group.key === activeGroup) ?? groups[0],
    [groups, activeGroup],
  );

  const options = useMemo(() => {
    const list = currentGroup?.options ?? [];
    const needle = query.trim().toLowerCase();
    if (!needle) return list;
    return list.filter(option =>
      option.label.toLowerCase().includes(needle) || option.value.toLowerCase().includes(needle),
    );
  }, [currentGroup, query]);

  const selected = useMemo(
    () => (currentGroup ? values[currentGroup.key] ?? [] : []),
    [currentGroup, values],
  );

  const searchable = currentGroup?.searchable ?? (currentGroup?.options.length ?? 0) > 7;

  // Reset transient panel state whenever it closes.
  const close = useCallback(() => {
    setOpen(false);
    setQuery('');
    setFocusIdx(0);
    triggerRef.current?.focus();
  }, []);

  // Recompute the anchor point on open, flip above when the viewport runs out.
  useLayoutEffect(() => {
    if (!open) return;
    const compute = () => {
      const trigger = triggerRef.current;
      if (!trigger) return;
      const isMobile = window.innerWidth < 640;
      setMobile(isMobile);
      if (isMobile) {
        setPos(null);
        return;
      }
      const rect = trigger.getBoundingClientRect();
      const gap = 6;
      const width = Math.min(PANEL_WIDTH, window.innerWidth - 16);
      let left = align === 'right' ? rect.right - width : rect.left;
      left = Math.max(8, Math.min(left, window.innerWidth - width - 8));
      const spaceBelow = window.innerHeight - rect.bottom - gap - 8;
      const spaceAbove = rect.top - gap - 8;
      const flip = spaceBelow < 220 && spaceAbove > spaceBelow;
      setPos({
        top: flip ? Math.max(8, rect.top - gap - Math.min(420, spaceAbove)) : rect.bottom + gap,
        left,
        maxHeight: Math.max(180, (flip ? spaceAbove : spaceBelow) - 8),
      });
    };
    compute();
    const onMove = () => compute();
    window.addEventListener('resize', onMove);
    window.addEventListener('scroll', onMove, true);
    return () => {
      window.removeEventListener('resize', onMove);
      window.removeEventListener('scroll', onMove, true);
    };
  }, [open, align]);

  // Dismiss on outside click / Escape.
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      const target = event.target as Node;
      if (panelRef.current?.contains(target) || triggerRef.current?.contains(target)) return;
      setOpen(false);
      setQuery('');
      setFocusIdx(0);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation();
        close();
      }
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown, true);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown, true);
    };
  }, [open, close]);

  useEffect(() => {
    if (open) searchRef.current?.focus();
  }, [open, activeGroup]);

  // Keep the highlighted option inside the list after filtering.
  useEffect(() => {
    setFocusIdx(prev => (options.length ? Math.min(prev, options.length - 1) : 0));
  }, [options.length]);

  const handlePanelKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setFocusIdx(prev => (options.length ? (prev + 1) % options.length : 0));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setFocusIdx(prev => (options.length ? (prev - 1 + options.length) % options.length : 0));
    } else if (event.key === 'Enter' && options[focusIdx]) {
      event.preventDefault();
      onToggle(currentGroup.key, options[focusIdx].value);
    }
  };

  const panel = open && (
    <div
      ref={panelRef}
      className={`glass-dropdown filter-menu-panel ${mobile ? 'filter-menu-panel-mobile' : ''}`}
      role="dialog"
      aria-label={triggerLabel}
      onKeyDown={handlePanelKeyDown}
      style={mobile ? undefined : pos ? { top: pos.top, left: pos.left, maxHeight: pos.maxHeight } : { display: 'none' }}
    >
      <div className="filter-menu-groups" role="tablist" aria-orientation="vertical" aria-label="Filter fields">
        {groups.map(group => {
          const count = (values[group.key] ?? []).length;
          const active = group.key === currentGroup?.key;
          return (
            <button
              key={group.key}
              type="button"
              role="tab"
              aria-selected={active}
              className="filter-menu-group"
              data-active={active ? 'true' : 'false'}
              onClick={() => {
                setActiveGroup(group.key);
                setQuery('');
                setFocusIdx(0);
              }}
            >
              <span className="truncate">{group.label}</span>
              {count > 0 && <span className="filter-menu-group-count">{count}</span>}
            </button>
          );
        })}
      </div>

      <div className="filter-menu-values">
        {searchable && (
          <input
            ref={searchRef}
            type="text"
            value={query}
            onChange={event => {
              setQuery(event.target.value);
              setFocusIdx(0);
            }}
            placeholder={`Type to find ${currentGroup?.label.toLowerCase()}...`}
            className="filter-menu-search"
            aria-label={`Search ${currentGroup?.label ?? 'options'}`}
          />
        )}

        <div role="listbox" aria-multiselectable={currentGroup?.multi !== false} className="filter-menu-list">
          {options.length === 0 && (
            <p className="px-3 py-4 text-[12px] text-[var(--text-tertiary)]">No matches</p>
          )}
          {options.map((option, index) => {
            const checked = selected.includes(option.value);
            const empty = option.count !== undefined && option.count === 0 && !checked;
            return (
              <button
                key={`${currentGroup?.key}-${option.value}`}
                type="button"
                role="option"
                aria-selected={checked}
                disabled={option.disabled || empty}
                data-focused={focusIdx === index ? 'true' : 'false'}
                onMouseMove={() => setFocusIdx(index)}
                onClick={() => onToggle(currentGroup.key, option.value)}
                className="filter-menu-option"
              >
                <span className={`filter-menu-mark ${checked ? 'filter-menu-mark-on' : ''}`} aria-hidden="true">
                  {checked && (
                    <svg className="w-2.5 h-2.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                    </svg>
                  )}
                </span>
                {option.tone && <span className={`filter-pill-dot ${DOT_CLASS[option.tone]}`} aria-hidden="true" />}
                <span className="truncate flex-1">{option.label}</span>
                {option.count !== undefined && (
                  <span className="filter-menu-count" data-empty={empty ? 'true' : 'false'}>{option.count}</span>
                )}
              </button>
            );
          })}
        </div>

        {selected.length > 0 && (
          <div className="filter-menu-footer">
            <button type="button" className="filter-menu-clear" onClick={() => onClearGroup(currentGroup.key)}>
              Clear {currentGroup.label.toLowerCase()}
            </button>
          </div>
        )}
      </div>
    </div>
  );

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        className={`filter-menu-trigger ${className}`}
        data-active={open || activeCount > 0 ? 'true' : 'false'}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => (open ? close() : setOpen(true))}
      >
        {variant === 'sort' ? (
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 7h11M3 12h8M3 17h5M17 8v9m0 0l3-3m-3 3l-3-3" />
          </svg>
        ) : (
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
          </svg>
        )}
        <span>{triggerLabel}</span>
        {activeCount > 0 && <span className="filter-menu-badge">{activeCount}</span>}
        <svg className="w-3 h-3 opacity-60" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {typeof document !== 'undefined' && open && createPortal(panel, document.body)}
    </>
  );
}
