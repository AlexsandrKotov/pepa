interface StatusBadgeProps {
  status: string;
  label?: string;
  pulse?: boolean;
  size?: 'sm' | 'md';
}

/**
 * Maps common status strings to semantic colors.
 * Supports: running/healthy/active/connected, failed/error/down, pending/waiting/progressing,
 * degraded/warning, unknown/default, stopped/inactive.
 */
function getStatusStyle(status: string): { bg: string; color: string; dot: string } {
  const s = status.toLowerCase();

  // Active / healthy
  if (['running', 'healthy', 'active', 'connected', 'deployed', 'success', 'succeeded', 'online'].includes(s)) {
    return { bg: 'bg-[var(--success-subtle)]', color: 'text-[var(--success)]', dot: 'bg-[var(--success)]' };
  }
  // Failed / error
  if (['failed', 'error', 'down', 'disconnected', 'crashloopbackoff', 'evicted'].includes(s)) {
    return { bg: 'bg-[var(--danger-subtle)]', color: 'text-[var(--danger)]', dot: 'bg-[var(--danger)]' };
  }
  // Pending / in-progress
  if (['pending', 'waiting', 'progressing', 'deploying', 'building', 'queued', 'created'].includes(s)) {
    return { bg: 'bg-[var(--info-subtle)]', color: 'text-[var(--info)]', dot: 'bg-[var(--info)]' };
  }
  // Degraded / warning
  if (['degraded', 'warning', 'unstable', 'flapping'].includes(s)) {
    return { bg: 'bg-[var(--warning-subtle)]', color: 'text-[var(--warning)]', dot: 'bg-[var(--warning)]' };
  }
  // Stopped / inactive
  if (['stopped', 'inactive', 'disabled', 'suspended', 'terminated', 'cancelled'].includes(s)) {
    return { bg: 'bg-[var(--border-light)]', color: 'text-[var(--text-tertiary)]', dot: 'bg-[var(--text-tertiary)]' };
  }
  // Default / unknown
  return { bg: 'bg-[var(--border-light)]', color: 'text-[var(--text-secondary)]', dot: 'bg-[var(--text-tertiary)]' };
}

function formatLabel(status: string): string {
  return status
    .replace(/_/g, ' ')
    .replace(/-/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase());
}

export default function StatusBadge({ status, label, pulse = false, size = 'md' }: StatusBadgeProps) {
  const style = getStatusStyle(status);
  const displayLabel = label || formatLabel(status);
  const isPulsing = pulse && ['running', 'healthy', 'active', 'connected', 'deployed', 'progressing', 'deploying', 'building'].includes(status.toLowerCase());

  const sizeClasses = size === 'sm'
    ? 'px-1.5 py-0.5 text-[10px] gap-1'
    : 'px-2 py-0.5 text-[11px] gap-1.5';

  const dotSize = size === 'sm' ? 'w-1.5 h-1.5' : 'w-1.5 h-1.5';

  return (
    <span className={`inline-flex items-center ${sizeClasses} font-medium rounded-[6px] ${style.bg} ${style.color}`}>
      <span className="relative flex items-center justify-center">
        <span className={`${dotSize} rounded-full ${style.dot}`} />
        {isPulsing && (
          <span className={`absolute inset-0 ${dotSize} rounded-full ${style.dot} animate-ping opacity-40`} />
        )}
      </span>
      {displayLabel}
    </span>
  );
}
