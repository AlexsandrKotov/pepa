'use client';

import { useCallback, useEffect, useRef, useState } from 'react';

interface SearchInputProps {
  /** Debounced value owned by the page (mirrors the URL). */
  value: string;
  /** Called after the debounce window with the new query. */
  onCommit: (value: string) => void;
  placeholder?: string;
  label?: string;
  /** Debounce window, ms. Project standard is 300. */
  delayMs?: number;
  /** Renders the in-flight indicator inside the field. */
  loading?: boolean;
  /** Register "/" as the focus shortcut. Only one field per page may enable this. */
  hotkey?: boolean;
  className?: string;
}

/**
 * Unified search field: leading icon, debounced commit, in-field clear button,
 * "/" focus shortcut and an inline in-flight indicator.
 * Replaces the 20+ hand-rolled `input !pl-9` + inline <svg> copies.
 */
export default function SearchInput({
  value,
  onCommit,
  placeholder = 'Search...',
  label = 'Search',
  delayMs = 300,
  loading = false,
  hotkey = true,
  className = '',
}: SearchInputProps) {
  const [draft, setDraft] = useState(value);
  const inputRef = useRef<HTMLInputElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastCommittedRef = useRef(value);
  const commitRef = useRef(onCommit);
  commitRef.current = onCommit;

  // Adopt external updates (Clear all, browser Back) without fighting the local draft.
  useEffect(() => {
    if (value !== lastCommittedRef.current) {
      lastCommittedRef.current = value;
      setDraft(value);
    }
  }, [value]);

  useEffect(() => () => {
    if (timerRef.current) clearTimeout(timerRef.current);
  }, []);

  const commit = useCallback((next: string) => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      lastCommittedRef.current = next;
      commitRef.current(next);
    }, delayMs);
  }, [delayMs]);

  const handleChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setDraft(event.target.value);
    commit(event.target.value);
  };

  const clear = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    lastCommittedRef.current = '';
    setDraft('');
    commitRef.current('');
    inputRef.current?.focus();
  }, []);

  // "/" focuses the field, Escape clears it — GitHub/Linear muscle memory.
  useEffect(() => {
    if (!hotkey) return;
    const handler = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typingInField =
        !!target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
      if (event.key === '/' && !typingInField && !event.metaKey && !event.ctrlKey && !event.altKey) {
        event.preventDefault();
        inputRef.current?.focus();
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [hotkey]);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      event.stopPropagation();
      if (draft) clear();
      else inputRef.current?.blur();
    }
    if (event.key === 'Enter') {
      if (timerRef.current) clearTimeout(timerRef.current);
      lastCommittedRef.current = draft;
      commitRef.current(draft);
    }
  };

  return (
    <div className={`filter-search ${className}`}>
      <svg className="filter-search-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
        <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
      </svg>
      <input
        ref={inputRef}
        type="search"
        value={draft}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        aria-label={label}
        className="filter-search-input"
      />
      {loading && <span className="filter-search-busy" aria-hidden="true" />}
      {draft ? (
        <button type="button" onClick={clear} className="filter-search-clear" aria-label="Clear search">
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      ) : (
        hotkey && <kbd className="filter-kbd" aria-hidden="true">/</kbd>
      )}
    </div>
  );
}
