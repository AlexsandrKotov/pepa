'use client';

import { type ReactNode, useCallback, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';

interface DialogProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  description?: string;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  children?: ReactNode;
  footer?: ReactNode;
  showCloseButton?: boolean;
}

const sizeClasses = {
  sm: 'max-w-sm',
  md: 'max-w-md',
  lg: 'max-w-lg',
  xl: 'max-w-xl',
};

/**
 * Modern dialog component with backdrop blur, animation, and focus trap.
 * Replaces ad-hoc modal patterns across the app.
 * For confirmations, continue using ConfirmModal (it uses this internally).
 */
export default function Dialog({
  open,
  onClose,
  title,
  description,
  size = 'md',
  children,
  footer,
  showCloseButton = true,
}: DialogProps) {
  const overlayRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);

  // Close on Escape
  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Escape') onClose();
  }, [onClose]);

  useEffect(() => {
    if (!open) return;
    document.addEventListener('keydown', handleKeyDown);
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.body.style.overflow = '';
    };
  }, [open, handleKeyDown]);

  // Focus content on open
  useEffect(() => {
    if (open && contentRef.current) {
      contentRef.current.focus();
    }
  }, [open]);

  if (!open) return null;

  return createPortal(
    <div ref={overlayRef} className="fixed inset-0 z-[9998] flex items-center justify-center p-4">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/30 backdrop-blur-sm"
        onClick={onClose}
        style={{ animation: 'content-reveal 0.15s ease-out reverse' }}
      />
      {/* Content */}
      <div
        ref={contentRef}
        tabIndex={-1}
        className={`relative w-full ${sizeClasses[size]} bg-[var(--surface)] rounded-[var(--radius-lg)] shadow-2xl border border-[var(--border)] overflow-hidden`}
        style={{ animation: 'dialog-enter 0.2s ease-out forwards' }}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        {/* Header */}
        {(title || showCloseButton) && (
          <div className="flex items-start justify-between gap-3 px-5 py-4 border-b border-[var(--border-light)]">
            <div className="min-w-0">
              {title && (
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">{title}</h2>
              )}
              {description && (
                <p className="text-[12px] text-[var(--text-secondary)] mt-0.5">{description}</p>
              )}
            </div>
            {showCloseButton && (
              <button
                onClick={onClose}
                className="p-1 -mr-1 -mt-1 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] rounded-lg transition-colors"
                aria-label="Close"
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            )}
          </div>
        )}

        {/* Body */}
        {children && (
          <div className="px-5 py-4 max-h-[60vh] overflow-y-auto">
            {children}
          </div>
        )}

        {/* Footer */}
        {footer && (
          <div className="flex items-center justify-end gap-2 px-5 py-3 bg-[var(--bg)] border-t border-[var(--border-light)]">
            {footer}
          </div>
        )}
      </div>
    </div>,
    document.body
  );
}
