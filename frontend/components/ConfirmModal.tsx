'use client';

import { type ReactNode, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';

interface ConfirmModalProps {
  open: boolean;
  title: string;
  description: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: 'danger' | 'warning' | 'default';
  loading?: boolean;
  icon?: ReactNode;
  onConfirm: () => void;
  onCancel: () => void;
}

export default function ConfirmModal({
  open,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'default',
  loading = false,
  icon,
  onConfirm,
  onCancel,
}: ConfirmModalProps) {
  const handleEscape = useCallback(() => {
    if (!loading) onCancel();
  }, [loading, onCancel]);
  useEscapeKey(handleEscape, open);

  if (!open) return null;

  const confirmBtnClass =
    variant === 'danger'
      ? 'btn btn-danger'
      : variant === 'warning'
        ? 'px-3 py-1.5 text-[13px] font-medium rounded-md bg-amber-500 text-white hover:bg-amber-600'
        : 'btn btn-primary';

  return (
    <div className="fixed inset-0 z-[9998] flex items-center justify-center">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={onCancel} />
      <div className="relative bg-white rounded-xl shadow-2xl border border-[#e5e5e5] w-full max-w-sm mx-4 overflow-hidden glass-modal">
        <div className="p-5">
          {icon && (
            <div className="flex justify-center mb-3">{icon}</div>
          )}
          <h3 className="text-[15px] font-semibold text-[#171717] text-center mb-1">{title}</h3>
          <p className="text-[13px] text-[#525252] text-center">{description}</p>
        </div>
        <div className="flex items-center gap-2 px-5 py-3 bg-[#fafafa] border-t border-[#f0f0f0]">
          <button onClick={onCancel} disabled={loading} className="btn btn-secondary flex-1 justify-center">
            {cancelLabel}
          </button>
          <button onClick={onConfirm} disabled={loading} className={`${confirmBtnClass} flex-1 justify-center`}>
            {loading ? (
              <span className="flex items-center gap-2">
                <svg className="animate-spin w-3.5 h-3.5" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Processing...
              </span>
            ) : (
              confirmLabel
            )}
          </button>
        </div>
      </div>
    </div>
  );
}
