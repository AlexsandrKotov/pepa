'use client';

import { useState, useCallback, useRef, useEffect, type ReactNode, memo } from 'react';
import { createPortal } from 'react-dom';

interface PreviewPopoverProps {
  children: ReactNode;
  /** Content shown inside the preview popover */
  preview: ReactNode;
  /** Optional action buttons shown at the bottom of the preview */
  actions?: ReactNode;
  /** Delay in ms before the preview appears (default: 500) */
  delay?: number;
  /** Disable the preview */
  disabled?: boolean;
}

/**
 * 3D Touch / long-press preview popover.
 * Hold down on the trigger element to show an enlarged preview.
 * Uses spring "pop" animation for iOS-like feel.
 */
function PreviewPopoverInner({
  children,
  preview,
  actions,
  delay = 500,
  disabled = false,
}: PreviewPopoverProps) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState({ x: 0, y: 0 });
  const triggerRef = useRef<HTMLDivElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const popoverRef = useRef<HTMLDivElement>(null);

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const openPreview = useCallback(() => {
    if (disabled) return;
    // Calculate position based on trigger element
    if (triggerRef.current) {
      const rect = triggerRef.current.getBoundingClientRect();
      const popoverW = 320;
      const popoverH = 240;

      // Prefer showing above the element, fall back to below
      let y = rect.top - popoverH - 12;
      if (y < 8) {
        y = rect.bottom + 12;
      }

      // Center horizontally relative to trigger
      let x = rect.left + rect.width / 2 - popoverW / 2;
      // Clamp to viewport
      x = Math.max(8, Math.min(x, window.innerWidth - popoverW - 8));
      y = Math.max(8, Math.min(y, window.innerHeight - popoverH - 8));

      setPos({ x, y });
    }
    setOpen(true);
  }, [disabled]);

  const closePreview = useCallback(() => {
    setOpen(false);
  }, []);

  // Long press via pointer events
  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    if (disabled) return;
    clearTimer();
    timerRef.current = setTimeout(() => {
      openPreview();
    }, delay);
  }, [disabled, delay, clearTimer, openPreview]);

  const handlePointerUp = useCallback(() => {
    clearTimer();
  }, [clearTimer]);

  const handlePointerLeave = useCallback(() => {
    clearTimer();
  }, [clearTimer]);

  // Close on outside click or scroll
  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      const target = e.target;
      if (
        target instanceof Node &&
        !popoverRef.current?.contains(target) &&
        !triggerRef.current?.contains(target)
      ) {
        closePreview();
      }
    };
    const handleScroll = () => closePreview();

    document.addEventListener('mousedown', handleClick);
    window.addEventListener('scroll', handleScroll, true);
    return () => {
      document.removeEventListener('mousedown', handleClick);
      window.removeEventListener('scroll', handleScroll, true);
    };
  }, [open, closePreview]);

  // Escape to close
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closePreview();
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, closePreview]);

  return (
    <>
      <div
        ref={triggerRef}
        tabIndex={0}
        role="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        onPointerDown={handlePointerDown}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerLeave}
        onKeyDown={(e) => {
          if (disabled) return;
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            openPreview();
          }
        }}
        className="inline-block"
        style={{
          transition: 'transform 0.15s var(--ease-ios-spring)',
          transform: open ? 'scale(0.97)' : 'scale(1)',
        }}
      >
        {children}
      </div>

      {open && createPortal(
        <div
          ref={popoverRef}
          className="fixed z-[10000] w-80 rounded-2xl border border-[var(--border)] overflow-hidden"
          style={{
            left: pos.x,
            top: pos.y,
            background: 'rgba(255,255,255,0.92)',
            backdropFilter: 'blur(20px) saturate(1.5)',
            WebkitBackdropFilter: 'blur(20px) saturate(1.5)',
            boxShadow: '0 8px 40px rgba(0,0,0,0.15), 0 2px 8px rgba(0,0,0,0.06)',
            animation: 'preview-pop 0.25s var(--ease-ios-spring) forwards',
          }}
          role="dialog"
          aria-label="Preview"
        >
          {/* Preview content */}
          <div className="p-4 max-h-[60vh] overflow-y-auto">
            {preview}
          </div>

          {/* Actions */}
          {actions && (
            <div className="flex items-center gap-2 px-4 py-2.5 border-t border-[var(--border-light)] bg-[var(--bg)]/50">
              {actions}
            </div>
          )}
        </div>,
        document.body
      )}

      {/* Dark mode override */}
      {open && (
        <style>{`
          [data-theme="dark"] .fixed.z-\\[10000\\] {
            background: rgba(34,38,46,0.92) !important;
            box-shadow: 0 8px 40px rgba(0,0,0,0.4), 0 2px 8px rgba(0,0,0,0.2) !important;
          }
        `}</style>
      )}
    </>
  );
}

const PreviewPopover = memo(PreviewPopoverInner);
PreviewPopover.displayName = 'PreviewPopover';

export default PreviewPopover;
