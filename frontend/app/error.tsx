'use client';

import { useEffect } from 'react';
import Link from 'next/link';

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error('[PEPA] Page error:', error.message, error.stack);
  }, [error]);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate">
          <h1 className="page-title-modern">Something went wrong</h1>
          <p className="page-subtitle-modern">An unexpected error occurred while loading this page</p>
        </div>

        <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="card-body">
            <div className="flex items-start gap-4">
              <div className="w-10 h-10 rounded-full bg-red-500/10 flex items-center justify-center shrink-0">
                <svg className="w-5 h-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                </svg>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-[13px] text-[var(--text-primary)] font-medium mb-1">Error Details</p>
                <p className="text-[13px] text-[var(--text-secondary)]">
                  {error.message || 'An unexpected error occurred while loading this page.'}
                </p>
                {error.digest && (
                  <p className="text-[11px] text-[var(--text-tertiary)] mt-2 font-mono">Error ID: {error.digest}</p>
                )}
              </div>
            </div>
          </div>
          <div className="card-footer flex items-center gap-3">
            <button onClick={reset} className="btn btn-primary btn-sm">
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182" />
              </svg>
              Try again
            </button>
            <Link href="/" className="btn btn-secondary btn-sm">
              Go to Dashboard
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
