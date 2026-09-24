'use client';

import React from 'react';

interface ErrorBoundaryProps {
  children: React.ReactNode;
  fallback?: React.ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error: Error | null;
}

/**
 * Global error boundary to catch uncaught React errors and prevent full app crashes.
 * Displays a user-friendly error page with recovery options.
 */
export default class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('ErrorBoundary caught an error:', error, errorInfo);
  }

  handleReload = () => {
    window.location.reload();
  };

  handleGoHome = () => {
    window.location.href = '/';
  };

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <div className="flex items-center justify-center min-h-screen bg-[var(--bg)]">
          <div className="max-w-md w-full mx-6 bg-[var(--surface)] border border-[var(--border)] rounded-xl p-8 shadow-lg">
            <div className="text-center">
              <div className="w-16 h-16 mx-auto mb-4 rounded-full bg-red-100 dark:bg-red-900/20 flex items-center justify-center">
                <svg className="w-8 h-8 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                </svg>
              </div>
              <h2 className="text-xl font-semibold text-[var(--text-primary)] mb-2">Something went wrong</h2>
              <p className="text-sm text-[var(--text-secondary)] mb-6">
                An unexpected error occurred. You can try reloading or return to the home page.
              </p>
              {process.env.NODE_ENV === 'development' && this.state.error && (
                <details className="mb-6 text-left">
                  <summary className="text-xs font-mono text-[var(--text-tertiary)] cursor-pointer hover:text-[var(--text-secondary)]">
                    Error details (development only)
                  </summary>
                  <pre className="mt-2 p-3 bg-[var(--bg)] rounded-lg text-xs font-mono text-red-600 dark:text-red-400 overflow-auto max-h-40">
                    {this.state.error.message}
                    {'\n'}
                    {this.state.error.stack}
                  </pre>
                </details>
              )}
              <div className="flex gap-3">
                <button
                  onClick={this.handleReload}
                  className="flex-1 px-4 py-2 bg-[var(--accent)] text-white rounded-lg hover:opacity-90 transition-opacity text-sm font-medium"
                >
                  Reload page
                </button>
                <button
                  onClick={this.handleGoHome}
                  className="flex-1 px-4 py-2 bg-[var(--bg)] border border-[var(--border)] text-[var(--text-primary)] rounded-lg hover:bg-[var(--border-light)] transition-colors text-sm font-medium"
                >
                  Go home
                </button>
              </div>
            </div>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
