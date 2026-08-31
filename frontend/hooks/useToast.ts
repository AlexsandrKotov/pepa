'use client';

import { useState, useCallback, useRef } from 'react';

export type ToastType = 'success' | 'error' | 'info';

export interface ToastItem {
  id: string;
  message: string;
  type: ToastType;
  hint?: string;
  duration: number;
}

const MAX_VISIBLE = 3;
const DEFAULT_DURATION: Record<ToastType, number> = {
  success: 4000,
  error: 6000,
  info: 4000,
};

export function useToast() {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const counterRef = useRef(0);

  const removeToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id));
  }, []);

  const addToast = useCallback((message: string, type: ToastType, hint?: string) => {
    const id = `toast-${++counterRef.current}-${Date.now()}`;
    const duration = DEFAULT_DURATION[type];
    const toast: ToastItem = { id, message, type, hint, duration };

    setToasts(prev => {
      const next = [...prev, toast];
      // Enforce max visible: dismiss oldest if over limit
      if (next.length > MAX_VISIBLE) {
        return next.slice(next.length - MAX_VISIBLE);
      }
      return next;
    });

    // Auto-dismiss
    setTimeout(() => {
      setToasts(prev => prev.filter(t => t.id !== id));
    }, duration);

    return id;
  }, []);

  return { toasts, addToast, removeToast };
}
