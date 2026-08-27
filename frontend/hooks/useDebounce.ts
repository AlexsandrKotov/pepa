'use client';

import { useState, useEffect, useCallback } from 'react';

/**
 * Debounces a value by the given delay (ms).
 * Useful for search inputs to avoid firing API calls on every keystroke.
 */
export function useDebounce<T>(value: T, delay = 300): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);

  return debounced;
}

/**
 * Debounces a callback function.
 * Returns a stable function reference that won't change between renders.
 */
export function useDebouncedCallback<A extends unknown[]>(
  callback: (...args: A) => void,
  delay = 300,
): (...args: A) => void {
  const [timer, setTimer] = useState<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timer) clearTimeout(timer);
    };
  }, [timer]);

  return useCallback(
    (...args: A) => {
      if (timer) clearTimeout(timer);
      const t = setTimeout(() => callback(...args), delay);
      setTimer(t);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [callback, delay],
  );
}
