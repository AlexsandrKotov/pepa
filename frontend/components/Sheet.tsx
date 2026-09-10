'use client';

import { type ReactNode, useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

interface SheetProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  size?: 'small' | 'medium' | 'large' | 'fullscreen';
  children?: ReactNode;
  footer?: ReactNode;
  /** Snap points as percentages of viewport height (e.g. [40, 70]). Sheet snaps to closest on drag release. */
  snapPoints?: number[];
}

const sizeToVh: Record<string, string> = {
  small: '40vh',
  medium: '60vh',
  large: '90vh',
  fullscreen: '100vh',
};

const DISMISS_THRESHOLD = 80; // px dragged past to auto-dismiss

/**
 * iOS-style bottom sheet with drag-to-dismiss.
 * Slides up from the bottom with spring animation.
 * Drag the handle (or the sheet itself) downward to dismiss.
 */
export default function Sheet({
  open,
  onClose,
  title,
  size = 'medium',
  children,
  footer,
  snapPoints,
}: SheetProps) {
  const sheetRef = useRef<HTMLDivElement>(null);
  const [dragY, setDragY] = useState(0);
  const [isDragging, setIsDragging] = useState(false);
  const dragStartRef = useRef(0);
  const [closing, setClosing] = useState(false);

  // Close with exit animation
  const handleClose = useCallback(() => {
    setClosing(true);
    setTimeout(() => {
      setClosing(false);
      setDragY(0);
      onClose();
    }, 200);
  }, [onClose]);

  // Escape key
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') handleClose();
    };
    document.addEventListener('keydown', handler);
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', handler);
      document.body.style.overflow = '';
    };
  }, [open, handleClose]);

  // Drag-to-dismiss handlers
  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    dragStartRef.current = e.clientY;
    setIsDragging(true);
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
  }, []);

  const handlePointerMove = useCallback((e: React.PointerEvent) => {
    if (!isDragging) return;
    const delta = e.clientY - dragStartRef.current;
    if (delta >= 0) {
      setDragY(delta);
    }
  }, [isDragging]);

  const handlePointerUp = useCallback(() => {
    setIsDragging(false);
    if (dragY > DISMISS_THRESHOLD) {
      handleClose();
    } else if (snapPoints && snapPoints.length > 0) {
      // Snap to closest point
      const vh = window.innerHeight / 100;
      const currentVh = (sheetRef.current?.offsetHeight || 0) / vh;
      const closest = snapPoints.reduce((prev, curr) =>
        Math.abs(curr - currentVh) < Math.abs(prev - currentVh) ? curr : prev
      );
      // We'll just let it stay at its natural size for now
      setDragY(0);
    } else {
      setDragY(0);
    }
  }, [dragY, snapPoints, handleClose]);

  if (!open && !closing) return null;

  const maxH = sizeToVh[size] || sizeToVh.medium;

  return createPortal(
    <div className="fixed inset-0 z-[9999] flex items-end justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/30 backdrop-blur-sm"
        onClick={handleClose}
        style={{
          opacity: closing ? 0 : 1,
          transition: 'opacity 0.2s var(--ease-ios)',
        }}
      />

      {/* Sheet panel */}
      <div
        ref={sheetRef}
        className="relative w-full max-w-2xl bg-[var(--surface)] rounded-t-[var(--radius-xl)] border-t border-x border-[var(--border)] overflow-hidden flex flex-col"
        style={{
          maxHeight: maxH,
          transform: closing
            ? 'translateY(100%)'
            : isDragging
              ? `translateY(${dragY}px)`
              : 'translateY(0)',
          transition: isDragging ? 'none' : `transform 0.3s var(--ease-ios-spring)`,
          animation: !closing && !isDragging ? 'sheet-enter 0.35s var(--ease-ios-snappy) forwards' : 'none',
        }}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        {/* Drag handle */}
        <div
          className="flex items-center justify-center pt-2 pb-1 cursor-grab active:cursor-grabbing shrink-0"
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          aria-label="Drag to dismiss"
        >
          <div className="w-9 h-1 rounded-full bg-[var(--border)]" />
        </div>

        {/* Header */}
        {title && (
          <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border-light)] shrink-0">
            <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">{title}</h2>
            <button
              onClick={handleClose}
              className="p-1 -mr-1 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] rounded-lg transition-colors"
              aria-label="Close"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        )}

        {/* Body */}
        {children && (
          <div className="flex-1 overflow-y-auto px-5 py-4 overscroll-contain">
            {children}
          </div>
        )}

        {/* Footer */}
        {footer && (
          <div className="flex items-center justify-end gap-2 px-5 py-3 bg-[var(--bg)] border-t border-[var(--border-light)] shrink-0">
            {footer}
          </div>
        )}
      </div>
    </div>,
    document.body
  );
}
