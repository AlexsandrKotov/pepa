import type { ReactNode } from 'react';

interface ErrorBannerProps {
  variant?: 'error' | 'warning' | 'info';
  title: string;
  description?: string;
  onRetry?: () => void;
  onDismiss?: () => void;
  icon?: ReactNode;
  compact?: boolean;
}

const variantStyles = {
  error: {
    bg: 'bg-[var(--danger-subtle)]',
    border: 'border-[var(--danger)]/20',
    iconColor: 'text-[var(--danger)]',
    titleColor: 'text-[var(--danger)]',
  },
  warning: {
    bg: 'bg-[var(--warning-subtle)]',
    border: 'border-[var(--warning)]/20',
    iconColor: 'text-[var(--warning)]',
    titleColor: 'text-[var(--warning)]',
  },
  info: {
    bg: 'bg-[var(--info-subtle)]',
    border: 'border-[var(--info)]/20',
    iconColor: 'text-[var(--info)]',
    titleColor: 'text-[var(--info)]',
  },
};

const defaultIcons = {
  error: (
    <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
    </svg>
  ),
  warning: (
    <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
    </svg>
  ),
  info: (
    <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
    </svg>
  ),
};

export default function ErrorBanner({
  variant = 'error',
  title,
  description,
  onRetry,
  onDismiss,
  icon,
  compact = false,
}: ErrorBannerProps) {
  const s = variantStyles[variant];

  if (compact) {
    return (
      <div className={`flex items-center gap-2 px-3 py-2 rounded-[var(--radius-sm)] ${s.bg} ${s.border} border`}>
        <span className={s.iconColor}>{icon || defaultIcons[variant]}</span>
        <span className={`text-[13px] font-medium ${s.titleColor} flex-1`}>{title}</span>
        {onRetry && (
          <button onClick={onRetry} className="text-[12px] font-medium text-[var(--accent)] hover:underline">
            Retry
          </button>
        )}
        {onDismiss && (
          <button onClick={onDismiss} className="p-0.5 text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors">
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        )}
      </div>
    );
  }

  return (
    <div className={`card ${s.bg} ${s.border} border`}>
      <div className="p-4 flex items-start gap-3">
        <div className={`shrink-0 mt-0.5 ${s.iconColor}`}>
          {icon || defaultIcons[variant]}
        </div>
        <div className="flex-1 min-w-0">
          <h3 className={`text-[13px] font-semibold ${s.titleColor}`}>{title}</h3>
          {description && (
            <p className="text-[12px] text-[var(--text-secondary)] mt-1 leading-relaxed">{description}</p>
          )}
          <div className="flex items-center gap-3 mt-3">
            {onRetry && (
              <button
                onClick={onRetry}
                className="btn btn-sm btn-secondary"
              >
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.037 9.348H9.03m-4.993 0l3.18-3.182a8.25 8.25 0 0113.803 3.7M20.015 4.356v4.992" />
                </svg>
                Retry
              </button>
            )}
            {onDismiss && (
              <button
                onClick={onDismiss}
                className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors"
              >
                Dismiss
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
