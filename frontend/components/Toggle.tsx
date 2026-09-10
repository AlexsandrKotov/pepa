'use client';

import { useCallback } from 'react';

interface ToggleProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  size?: 'sm' | 'md' | 'lg';
  disabled?: boolean;
  label?: string;
  id?: string;
}

const sizeConfig = {
  sm: { track: 'w-7 h-4', thumb: 'w-3 h-3', translate: 'translate-x-3', label: 'text-[12px]' },
  md: { track: 'w-[44px] h-6', thumb: 'w-5 h-5', translate: 'translate-x-[22px]', label: 'text-[13px]' },
  lg: { track: 'w-14 h-8', thumb: 'w-6 h-6', translate: 'translate-x-7', label: 'text-[14px]' },
};

/**
 * iOS-style toggle switch with spring animation.
 * Uses role="switch" for accessibility.
 */
export default function Toggle({
  checked,
  onChange,
  size = 'md',
  disabled = false,
  label,
  id,
}: ToggleProps) {
  const cfg = sizeConfig[size];

  const handleClick = useCallback(() => {
    if (!disabled) onChange(!checked);
  }, [checked, disabled, onChange]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (disabled) return;
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onChange(!checked);
    }
  }, [checked, disabled, onChange]);

  const toggleId = id || `toggle-${Math.random().toString(36).slice(2, 8)}`;

  return (
    <div className="inline-flex items-center gap-2">
      <button
        id={toggleId}
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={handleClick}
        onKeyDown={handleKeyDown}
        className={`
          relative inline-flex shrink-0 items-center rounded-full border-0 cursor-pointer
          ${cfg.track}
          ${disabled ? 'opacity-40 cursor-not-allowed' : ''}
          transition-colors duration-200
        `}
        style={{
          background: checked ? 'var(--success)' : 'var(--border)',
          transitionTimingFunction: 'var(--ease-ios)',
        }}
      >
        {/* Thumb */}
        <span
          className={`
            inline-block rounded-full bg-white shadow-sm
            ${cfg.thumb}
            transition-transform
          `}
          style={{
            transform: checked ? cfg.translate : 'translateX(2px)',
            transitionDuration: '0.2s',
            transitionTimingFunction: checked ? 'var(--ease-ios-spring)' : 'var(--ease-ios)',
            boxShadow: '0 1px 3px rgba(0,0,0,0.15), 0 1px 2px rgba(0,0,0,0.06)',
          }}
        />
      </button>
      {label && (
        <label htmlFor={toggleId} className={`cursor-pointer select-none ${cfg.label} ${disabled ? 'text-[var(--text-tertiary)]' : 'text-[var(--text-primary)]'}`}>
          {label}
        </label>
      )}
    </div>
  );
}
