'use client';

import { useState, useCallback, useEffect, useRef, memo } from 'react';
import { createPortal } from 'react-dom';

export interface ContextMenuItem {
  key: string;
  label: string;
  icon?: string; // SVG path d
  destructive?: boolean;
  disabled?: boolean;
  separator?: boolean;
  onClick?: () => void;
}

interface ContextMenuProps {
  items: ContextMenuItem[];
  children: React.ReactNode;
  /** Optional: disable the context menu on this wrapper */
  disabled?: boolean;
}

interface MenuPosition {
  x: number;
  y: number;
}

/**
 * macOS-style context menu — right-click or long-press to open.
 * Positions at cursor, glassmorphism background, keyboard navigation.
 */
function ContextMenuInner({ items, children, disabled }: ContextMenuProps) {
  const [open, setOpen] = useState(false);
  const [pos, setPos] = useState<MenuPosition>({ x: 0, y: 0 });
  const [focusIdx, setFocusIdx] = useState(-1);
  const menuRef = useRef<HTMLDivElement>(null);
  const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const enabledItems = items.filter(i => !i.separator && !i.disabled);

  const openMenu = useCallback((x: number, y: number) => {
    // Clamp position so menu stays within viewport
    const menuW = 220;
    const menuH = items.length * 32 + 16;
    const clampedX = Math.min(x, window.innerWidth - menuW - 8);
    const clampedY = Math.min(y, window.innerHeight - menuH - 8);
    setPos({ x: Math.max(8, clampedX), y: Math.max(8, clampedY) });
    setOpen(true);
    setFocusIdx(-1);
  }, [items.length]);

  const closeMenu = useCallback(() => {
    setOpen(false);
    setFocusIdx(-1);
  }, []);

  // Right-click handler
  const handleContextMenu = useCallback((e: React.MouseEvent) => {
    if (disabled) return;
    e.preventDefault();
    openMenu(e.clientX, e.clientY);
  }, [disabled, openMenu]);

  // Long-press handlers for touch devices
  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    if (disabled) return;
    longPressTimer.current = setTimeout(() => {
      openMenu(e.clientX, e.clientY);
    }, 500);
  }, [disabled, openMenu]);

  const handlePointerUp = useCallback(() => {
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
  }, []);

  const handlePointerLeave = useCallback(() => {
    if (longPressTimer.current) {
      clearTimeout(longPressTimer.current);
      longPressTimer.current = null;
    }
  }, []);

  // Close on outside click
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      const target = e.target;
      if (target instanceof Node && !menuRef.current?.contains(target)) {
        closeMenu();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open, closeMenu]);

  // Keyboard navigation
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        closeMenu();
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        setFocusIdx(prev => {
          const next = prev + 1;
          return next >= enabledItems.length ? 0 : next;
        });
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setFocusIdx(prev => {
          const next = prev - 1;
          return next < 0 ? enabledItems.length - 1 : next;
        });
      } else if (e.key === 'Enter' && focusIdx >= 0) {
        e.preventDefault();
        const item = enabledItems[focusIdx];
        if (item?.onClick) {
          item.onClick();
          closeMenu();
        }
      }
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open, focusIdx, enabledItems, closeMenu]);

  // Scroll focused item into view
  useEffect(() => {
    if (open && focusIdx >= 0 && menuRef.current) {
      const focused = menuRef.current.querySelector(`[data-ctx-idx="${focusIdx}"]`) as HTMLElement | null;
      focused?.scrollIntoView({ block: 'nearest' });
    }
  }, [focusIdx, open]);

  return (
    <>
      <div
        onContextMenu={handleContextMenu}
        onPointerDown={handlePointerDown}
        onPointerUp={handlePointerUp}
        onPointerLeave={handlePointerLeave}
      >
        {children}
      </div>

      {open && createPortal(
        <div
          ref={menuRef}
          className="fixed z-[10000] min-w-[180px] max-w-[260px] py-1 rounded-xl border border-[var(--border)] overflow-hidden"
          style={{
            left: pos.x,
            top: pos.y,
            background: 'rgba(255,255,255,0.88)',
            backdropFilter: 'blur(16px) saturate(1.5)',
            WebkitBackdropFilter: 'blur(16px) saturate(1.5)',
            boxShadow: '0 4px 24px rgba(0,0,0,0.12), 0 1px 4px rgba(0,0,0,0.04)',
            animation: 'context-menu-enter 0.15s var(--ease-ios-snappy) forwards',
          }}
          role="menu"
        >
          {/* Dark mode override */}
          <style>{`
            [data-theme="dark"] [role="menu"] {
              background: rgba(34,38,46,0.92) !important;
              box-shadow: 0 4px 24px rgba(0,0,0,0.4), 0 1px 4px rgba(0,0,0,0.2) !important;
            }
          `}</style>
          {items.map((item, idx) => {
            if (item.separator) {
              return <div key={`sep-${idx}`} className="h-px bg-[var(--border-light)] my-1 mx-2" />;
            }

            const enabledIdx = enabledItems.indexOf(item);
            const isFocused = enabledIdx === focusIdx;

            return (
              <button
                key={item.key}
                data-ctx-idx={enabledIdx}
                role="menuitem"
                disabled={item.disabled}
                className={`
                  w-full flex items-center gap-2.5 px-3 py-1.5 text-left text-[13px]
                  transition-colors duration-75
                  ${item.disabled ? 'text-[var(--text-tertiary)] cursor-not-allowed' : 'text-[var(--text-primary)] cursor-pointer'}
                  ${item.destructive ? '!text-[var(--danger)]' : ''}
                `}
                style={{
                  background: isFocused ? 'var(--accent)' : 'transparent',
                  color: isFocused ? '#fff' : undefined,
                  transitionTimingFunction: 'var(--ease-ios)',
                }}
                onMouseEnter={() => setFocusIdx(enabledIdx)}
                onMouseLeave={() => setFocusIdx(-1)}
                onClick={() => {
                  if (!item.disabled && item.onClick) {
                    item.onClick();
                    closeMenu();
                  }
                }}
              >
                {item.icon && (
                  <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
                  </svg>
                )}
                <span className="truncate">{item.label}</span>
              </button>
            );
          })}
        </div>,
        document.body
      )}
    </>
  );
}

const ContextMenu = memo(ContextMenuInner);
ContextMenu.displayName = 'ContextMenu';

export default ContextMenu;
