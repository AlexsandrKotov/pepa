'use client';

import type { ToastItem } from '@/hooks/useToast';

const typeStyles: Record<string, { bg: string; border: string; text: string; icon: string }> = {
  success: {
    bg: 'bg-emerald-600',
    border: 'border-emerald-500',
    text: 'text-white',
    icon: '\u2713', // ✓
  },
  error: {
    bg: 'bg-red-600',
    border: 'border-red-500',
    text: 'text-white',
    icon: '\u26A0', // ⚠
  },
  info: {
    bg: 'bg-blue-600',
    border: 'border-blue-500',
    text: 'text-white',
    icon: '\u2139', // ℹ
  },
};

export default function ToastContainer({ toasts, onRemove }: { toasts: ToastItem[]; onRemove: (id: string) => void }) {
  if (toasts.length === 0) return null;

  return (
    <div className="fixed top-6 left-1/2 -translate-x-1/2 z-[9999] flex flex-col gap-2 items-center pointer-events-none w-full px-4">
      {toasts.map(toast => {
        const style = typeStyles[toast.type] || typeStyles.info;
        return (
          <div
            key={toast.id}
            className={`pointer-events-auto rounded-xl border ${style.border} ${style.bg} px-4 py-3 flex items-center gap-3 shadow-2xl animate-in slide-in-from-top-4 fade-in duration-300 min-w-[280px] max-w-md`}
          >
            <span className={`text-base font-bold shrink-0 ${style.text}`}>{style.icon}</span>
            <div className="flex-1 min-w-0">
              <p className={`text-sm font-semibold ${style.text}`}>{toast.message}</p>
              {toast.hint && <p className="text-xs text-white/70 mt-0.5 break-words">{toast.hint}</p>}
            </div>
            <button
              onClick={() => onRemove(toast.id)}
              className="text-white/50 hover:text-white shrink-0 text-sm"
            >
              ✕
            </button>
          </div>
        );
      })}
    </div>
  );
}
