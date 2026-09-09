'use client';

import { useCallback } from 'react';

/**
 * Centralized error handling hook for consistent error management across the app.
 * Provides user-friendly error messages, logging, and session handling.
 */
export function useErrorHandler() {
  return useCallback((error: unknown, context?: string) => {
    const prefix = context ? `[${context}]` : '[App]';

    // Extract meaningful error message
    let message: string;
    if (error instanceof Error) {
      message = error.message;
    } else if (typeof error === 'string') {
      message = error;
    } else {
      message = 'An unexpected error occurred';
    }

    // Log to console for debugging
    console.error(`${prefix} Error:`, message, error);

    // Check for session expiration (401)
    if (error instanceof Response && error.status === 401) {
      // Session expired — the API client handles redirect automatically
      return;
    }

    // Return the message for components that want to display it
    return message;
  }, []);
}
