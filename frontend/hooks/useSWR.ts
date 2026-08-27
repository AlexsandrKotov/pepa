'use client';

import { useState, useEffect, useRef, useCallback } from 'react';

interface SWRConfig {
  /** Revalidate when window gains focus (default: true) */
  revalidateOnFocus?: boolean;
  /** Revalidate when network reconnects (default: true) */
  revalidateOnReconnect?: boolean;
  /** Polling interval in ms (0 = disabled) */
  refreshInterval?: number;
  /** Deduplicate requests within this many ms (default: 2000) */
  dedupInterval?: number;
}

interface SWRReturn<T> {
  data: T | undefined;
  error: Error | undefined;
  isLoading: boolean;
  isValidating: boolean;
  mutate: () => void;
}

// Global cache shared across all hook instances
const swrCache = new Map<string, { data: unknown; expiry: number; error?: Error }>();
const swrInflight = new Map<string, Promise<unknown>>();
const swrListeners = new Map<string, Set<() => void>>();

function getCacheEntry<T>(key: string): { data: T; error?: Error } | undefined {
  const entry = swrCache.get(key);
  if (entry && entry.expiry > Date.now()) {
    return { data: entry.data as T, error: entry.error };
  }
  return undefined;
}

function setCacheEntry(key: string, data: unknown, ttl: number, error?: Error) {
  swrCache.set(key, { data, expiry: Date.now() + ttl, error });
  // Notify all listeners for this key
  const listeners = swrListeners.get(key);
  if (listeners) {
    for (const fn of listeners) fn();
  }
}

async function swrFetch<T>(key: string, fetcher: () => Promise<T>, dedupMs: number, ttl: number): Promise<T> {
  // Check inflight dedup
  const pending = swrInflight.get(key);
  if (pending) return pending as Promise<T>;

  const promise = (async () => {
    try {
      const data = await fetcher();
      setCacheEntry(key, data, ttl);
      return data;
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      setCacheEntry(key, undefined, ttl, error);
      throw error;
    } finally {
      // Remove from inflight after dedup window
      setTimeout(() => swrInflight.delete(key), dedupMs);
    }
  })();

  swrInflight.set(key, promise);
  return promise;
}

/**
 * Lightweight SWR hook: returns cached data immediately, revalidates in background.
 * Deduplicates concurrent requests with the same key.
 */
export function useSWR<T>(key: string | null, fetcher: () => Promise<T>, config?: SWRConfig): SWRReturn<T> {
  const {
    revalidateOnFocus = true,
    revalidateOnReconnect = true,
    refreshInterval = 0,
    dedupInterval = 2000,
  } = config || {};

  const [, forceUpdate] = useState(0);
  const fetcherRef = useRef(fetcher);
  fetcherRef.current = fetcher;

  // Subscribe to cache changes for this key
  const rerender = useCallback(() => forceUpdate(n => n + 1), []);

  useEffect(() => {
    if (!key) return;
    if (!swrListeners.has(key)) swrListeners.set(key, new Set());
    swrListeners.get(key)!.add(rerender);
    return () => {
      const ls = swrListeners.get(key);
      if (ls) {
        ls.delete(rerender);
        if (ls.size === 0) swrListeners.delete(key);
      }
    };
  }, [key, rerender]);

  // Fetch / revalidate
  const doFetch = useCallback(() => {
    if (!key) return;
    swrFetch(key, () => fetcherRef.current(), dedupInterval, 30_000).catch(() => {});
  }, [key, dedupInterval]);

  // Initial fetch + stale-while-revalidate
  useEffect(() => {
    if (!key) return;
    const cached = getCacheEntry<T>(key);
    if (!cached) {
      doFetch();
    } else {
      // Stale: revalidate in background
      doFetch();
    }
  }, [key, doFetch]);

  // Focus revalidation
  useEffect(() => {
    if (!key || !revalidateOnFocus) return;
    const onFocus = () => doFetch();
    window.addEventListener('focus', onFocus);
    return () => window.removeEventListener('focus', onFocus);
  }, [key, revalidateOnFocus, doFetch]);

  // Reconnect revalidation
  useEffect(() => {
    if (!key || !revalidateOnReconnect) return;
    const onReconnect = () => doFetch();
    window.addEventListener('online', onReconnect);
    return () => window.removeEventListener('online', onReconnect);
  }, [key, revalidateOnReconnect, doFetch]);

  // Polling
  useEffect(() => {
    if (!key || !refreshInterval) return;
    const id = setInterval(doFetch, refreshInterval);
    return () => clearInterval(id);
  }, [key, refreshInterval, doFetch]);

  // Read current state from cache
  const cached = key ? getCacheEntry<T>(key) : undefined;
  const isLoading = !key || (!cached && swrInflight.has(key));
  const isValidating = key ? swrInflight.has(key) : false;

  return {
    data: cached?.data,
    error: cached?.error,
    isLoading,
    isValidating,
    mutate: doFetch,
  };
}
