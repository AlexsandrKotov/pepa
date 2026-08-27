'use client';

import useSWR, { type SWRConfiguration, type SWRResponse } from 'swr';
import type { SWRMutationConfiguration } from 'swr/mutation';
import useSWRMutation from 'swr/mutation';
import { getBase } from '@/lib/api';

/** Global SWR fetcher — resolves key to API path with optional query params. */
async function fetcher<T>(key: string): Promise<T> {
  const res = await fetch(`${getBase()}${key}`, { cache: 'no-store' });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `API error: ${res.status}`);
  }
  return res.json();
}

/**
 * PEPA data-fetching hook built on SWR.
 * Provides automatic caching, revalidation, deduplication, and stale-while-revalidate.
 *
 * @example
 *   const { data, error, isLoading, mutate } = useApi<ServiceListResponse>('/api/v1/services');
 *   const { data } = useApi<ServiceListResponse>('/api/v1/services', { search: 'payment' });
 */
export function useApi<T>(
  path: string | null,
  params?: Record<string, string>,
  config?: SWRConfiguration,
): SWRResponse<T> {
  const key = path
    ? params
      ? `${path}?${new URLSearchParams(params).toString()}`
      : path
    : null;

  return useSWR<T>(key, fetcher, {
    revalidateOnFocus: false,
    revalidateIfStale: true,
    revalidateOnReconnect: true,
    dedupingInterval: 5000,
    ...config,
  });
}

/**
 * Mutation hook for POST/PUT/PATCH/DELETE operations with optimistic updates.
 */
export function useApiMutation<TData = unknown, TArg = unknown>(
  path: string,
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE' = 'POST',
  config?: SWRMutationConfiguration<TData, Error, string, TArg>,
) {
  async function fetcher(url: string, { arg }: { arg: TArg }) {
    const res = await fetch(`${getBase()}${url}`, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(arg),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `API error: ${res.status}`);
    }
    if (res.status === 204) return undefined as TData;
    return res.json();
  }

  return useSWRMutation<TData, Error, string, TArg>(path, fetcher, config);
}
