import { useEffect, useRef, useCallback, useState } from 'react';

export interface SSEEvent {
  type: string;
  data: Record<string, unknown>;
}

interface UseSSEOptions {
  /** Filter events by type prefix (e.g. 'deployment.', 'gitops.') */
  filterTypes?: string[];
  /** Whether the SSE connection should be active */
  enabled?: boolean;
  /** Called for every matching event */
  onEvent?: (event: SSEEvent) => void;
}

/**
 * Hook for consuming the PEPA real-time SSE event stream.
 * Connects to /api/v1/events/stream and dispatches events by type.
 *
 * Event types follow the pattern: "resource.action" (e.g. "deployment.created",
 * "gitops.synced", "drift.detected"). The filterTypes option accepts prefixes
 * so passing ['deployment', 'gitops'] matches any event starting with those.
 */
export function useSSEStream({ filterTypes, enabled = true, onEvent }: UseSSEOptions = {}) {
  const [connected, setConnected] = useState(false);
  const [lastEvent, setLastEvent] = useState<SSEEvent | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  const connect = useCallback(() => {
    if (esRef.current) return;
    if (!enabled) return;

    // Build the SSE URL — the browser sends cookies automatically
    const url = '/api/v1/events/stream';
    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => {
      setConnected(false);
      // EventSource auto-reconnects by default
    };

    // Catch-all: the server sends events with a "type" field in the data
    es.onmessage = (msg) => {
      try {
        const parsed = JSON.parse(msg.data) as SSEEvent;
        const eventType = parsed.type || 'unknown';

        // Apply type filter
        if (filterTypes && filterTypes.length > 0) {
          const matches = filterTypes.some(prefix => eventType.startsWith(prefix));
          if (!matches) return;
        }

        setLastEvent(parsed);
        onEventRef.current?.(parsed);
      } catch {
        // Non-JSON data, ignore
      }
    };

    // Also listen for named events (server sends "event: <type>")
    const namedTypes = [
      'deployment.created', 'deployment.updated', 'deployment.failed',
      'gitops.synced', 'gitops.reconciled',
      'drift.detected', 'drift.resolved',
      'pipeline.started', 'pipeline.completed', 'pipeline.failed',
    ];
    for (const type of namedTypes) {
      es.addEventListener(type, (msg) => {
        try {
          const parsed = JSON.parse((msg as MessageEvent).data) as SSEEvent;
          parsed.type = type;

          if (filterTypes && filterTypes.length > 0) {
            const matches = filterTypes.some(prefix => type.startsWith(prefix));
            if (!matches) return;
          }

          setLastEvent(parsed);
          onEventRef.current?.(parsed);
        } catch {
          // ignore
        }
      });
    }
  }, [enabled, filterTypes]);

  const disconnect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
      setConnected(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) {
      connect();
    }
    return () => disconnect();
  }, [enabled, connect, disconnect]);

  return { connected, lastEvent, disconnect, reconnect: connect };
}

/**
 * Hook that auto-refreshes data when relevant SSE events arrive.
 * Pass a refresh function and the event types that should trigger it.
 */
export function useSSERefresh(triggerTypes: string[], refresh: () => void) {
  useSSEStream({
    filterTypes: triggerTypes,
    onEvent: () => refresh(),
  });
}
