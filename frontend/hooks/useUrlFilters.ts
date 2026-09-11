'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

/**
 * URL-as-state filter store.
 *
 * The query string is the single source of truth for list filters, so views are
 * shareable, bookmarkable and survive the browser Back button (every discrete
 * filter change is a pushState entry; keystrokes and page resets replace).
 *
 * Query params that the page does not declare (e.g. `id`, `tab`) are always kept
 * untouched, so this hook can coexist with detail-view routing.
 *
 * `ready` is false until the first client-side read of location.search, which
 * lets list pages skip their initial fetch instead of firing an unfiltered request.
 */

export interface UseUrlFiltersOptions {
  /** Query keys owned by the page that hold a single value. */
  single?: string[];
  /** Query keys owned by the page that hold comma-separated values. */
  multi?: string[];
  /** Key used for the page number; defaults to `page`. */
  pageKey?: string;
  /** Key used for the page size; defaults to `per_page`. */
  perPageKey?: string;
  defaultPerPage?: number;
  /**
   * Keys that live in the URL with the filters (sorting, density, view mode) but
   * are not filters: they stay untouched by clear() and are left out of activeCount.
   */
  keepOnClear?: string[];
}

export interface UrlFilters {
  /** True once location.search has been read on the client. */
  ready: boolean;
  /** Raw value of a single-value key ('' when unset). */
  get: (key: string) => string;
  /** Values of a multi-value key (empty array when unset). */
  getAll: (key: string) => string[];
  /** True when the key holds at least one value. */
  has: (key: string) => boolean;
  /** Set (or clear with '') a single-value key. Resets the page. */
  set: (key: string, value: string) => void;
  /** Add/remove a value on a multi-value key. Resets the page. */
  toggle: (key: string, value: string) => void;
  /** Drop every value of a key. Resets the page. */
  remove: (key: string) => void;
  /** Drop all filter keys, keepOnClear keys aside (pagination and foreign params survive). */
  clear: () => void;
  page: number;
  setPage: (page: number) => void;
  perPage: number;
  setPerPage: (perPage: number) => void;
  /** Number of active values across filter keys — drives the "Filters (n)" badge. */
  activeCount: number;
  /** Filter keys currently holding a value, in declaration order. */
  activeKeys: string[];
  /** Ready-to-send API params (empty keys omitted, multi values joined by comma). */
  toApiParams: () => Record<string, string>;
}

function parseSearch(search: string, multi: Set<string>, pageKey: string, perPageKey: string) {
  const params = new URLSearchParams(search);
  const values: Record<string, string[]> = {};
  for (const key of [...multi]) {
    const raw = params.get(key);
    values[key] = raw ? raw.split(',').map(v => v.trim()).filter(Boolean) : [];
  }
  const page = Number(params.get(pageKey));
  const perPage = Number(params.get(perPageKey));
  return {
    values,
    page: Number.isFinite(page) && page > 0 ? page : 1,
    perPage: Number.isFinite(perPage) && perPage > 0 ? perPage : 0,
  };
}

export function useUrlFilters(options: UseUrlFiltersOptions): UrlFilters {
  const { single = [], multi = [], pageKey = 'page', perPageKey = 'per_page', defaultPerPage = 20, keepOnClear = [] } = options;

  // Key lists are memoised by their joined content: callers pass fresh array
  // literals on every render, and any identity churn here would re-create the
  // param builders and put list pages into a refetch loop.
  const singleSig = useMemo(() => single.join(','), [single]);
  const multiSig = useMemo(() => multi.join(','), [multi]);
  const keepSig = useMemo(() => keepOnClear.join(','), [keepOnClear]);
  const singleKeys = useMemo(() => (singleSig ? singleSig.split(',').filter(k => !multiSig.split(',').includes(k)) : []), [singleSig, multiSig]);
  const multiKeys = useMemo(() => (multiSig ? multiSig.split(',') : []), [multiSig]);
  const keepKeys = useMemo(() => (keepSig ? keepSig.split(',') : []), [keepSig]);
  const keepSet = useMemo(() => new Set(keepKeys), [keepKeys]);
  const multiSet = useMemo(() => new Set(multiKeys), [multiKeys]);
  const filterKeys = useMemo(() => [...singleKeys, ...multiKeys], [singleKeys, multiKeys]);

  const [values, setValues] = useState<Record<string, string[]>>({});
  const [page, setPageState] = useState(1);
  const [perPage, setPerPageState] = useState(defaultPerPage);
  const [ready, setReady] = useState(false);

  const valuesRef = useRef(values);
  valuesRef.current = values;
  const pageRef = useRef(page);
  pageRef.current = page;
  const perPageRef = useRef(perPage);
  perPageRef.current = perPage;

  // First client read of the URL; everything before this is unhydrated.
  useEffect(() => {
    const parsed = parseSearch(window.location.search, multiSet, pageKey, perPageKey);
    setValues(prev => ({ ...prev, ...parsed.values }));
    setPageState(parsed.page);
    if (parsed.perPage) setPerPageState(parsed.perPage);
    setReady(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [singleSig, multiSig]);

  const write = useCallback(
    (nextValues: Record<string, string[]>, nextPage: number, nextPerPage: number, push: boolean) => {
      setValues(nextValues);
      setPageState(nextPage);
      setPerPageState(nextPerPage);

      const params = new URLSearchParams(window.location.search);
      for (const key of filterKeys) {
        const vals = nextValues[key] ?? [];
        if (vals.length) params.set(key, vals.join(','));
        else params.delete(key);
      }
      if (nextPage > 1) params.set(pageKey, String(nextPage));
      else params.delete(pageKey);
      if (nextPerPage && nextPerPage !== defaultPerPage) params.set(perPageKey, String(nextPerPage));
      else params.delete(perPageKey);

      const qs = params.toString();
      const url = `${window.location.pathname}${qs ? `?${qs}` : ''}${window.location.hash}`;
      if (push) window.history.pushState(null, '', url);
      else window.history.replaceState(null, '', url);
    },
    [filterKeys, pageKey, perPageKey, defaultPerPage],
  );

  // Back / forward restores the recorded view.
  useEffect(() => {
    const onPopState = () => {
      const parsed = parseSearch(window.location.search, multiSet, pageKey, perPageKey);
      setValues(prev => ({ ...prev, ...parsed.values }));
      setPageState(parsed.page);
      if (parsed.perPage) setPerPageState(parsed.perPage);
    };
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, [multiSet, pageKey, perPageKey]);

  const commit = useCallback(
    (nextValues: Record<string, string[]>, opts?: { keepPage?: boolean; push?: boolean }) => {
      const keepPage = opts?.keepPage ?? false;
      write(nextValues, keepPage ? pageRef.current : 1, perPageRef.current, opts?.push ?? true);
    },
    [write],
  );

  const get = useCallback((key: string) => (values[key] ?? [])[0] ?? '', [values]);

  const getAll = useCallback((key: string) => values[key] ?? [], [values]);

  const has = useCallback((key: string) => (values[key] ?? []).length > 0, [values]);

  const set = useCallback(
    (key: string, value: string) => {
      const current = valuesRef.current;
      commit({ ...current, [key]: value ? [value] : [] });
    },
    [commit],
  );

  const toggle = useCallback(
    (key: string, value: string) => {
      const current = valuesRef.current;
      const existing = current[key] ?? [];
      const next = existing.includes(value)
        ? existing.filter(v => v !== value)
        : [...existing, value];
      commit({ ...current, [key]: next });
    },
    [commit],
  );

  const remove = useCallback(
    (key: string) => {
      if (!(valuesRef.current[key] ?? []).length) return;
      commit({ ...valuesRef.current, [key]: [] });
    },
    [commit],
  );

  const clear = useCallback(() => {
    const cleared: Record<string, string[]> = {};
    for (const key of filterKeys) {
      if (!keepSet.has(key)) cleared[key] = [];
    }
    commit({ ...valuesRef.current, ...cleared });
  }, [commit, filterKeys, keepSet]);

  const setPage = useCallback((next: number) => write(valuesRef.current, next, perPageRef.current, true), [write]);

  const setPerPage = useCallback(
    (next: number) => write(valuesRef.current, 1, next, false),
    [write],
  );

  const activeKeys = useMemo(
    () => filterKeys.filter(key => !keepSet.has(key) && (values[key] ?? []).length > 0),
    [filterKeys, keepSet, values],
  );
  const activeCount = activeKeys.reduce((sum, key) => sum + (values[key] ?? []).length, 0);

  const toApiParams = useCallback(() => {
    const params: Record<string, string> = {};
    for (const key of filterKeys) {
      const vals = values[key] ?? [];
      if (vals.length) params[key] = vals.join(',');
    }
    return params;
  }, [filterKeys, values]);

  return {
    ready,
    get,
    getAll,
    has,
    set,
    toggle,
    remove,
    clear,
    page,
    setPage,
    perPage,
    setPerPage,
    activeCount,
    activeKeys,
    toApiParams,
  };
}
