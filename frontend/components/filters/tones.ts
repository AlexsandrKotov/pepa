import type { Tone } from '@/lib/filter-labels';

/** Semantic tone -> design-token classes. No hex/Tailwind palette values here. */
export const DOT_CLASS: Record<Tone, string> = {
  success: 'bg-[var(--success)]',
  danger: 'bg-[var(--danger)]',
  warning: 'bg-[var(--warning)]',
  info: 'bg-[var(--info)]',
  accent: 'bg-[var(--accent)]',
  muted: 'bg-[var(--text-tertiary)]',
};

export const TEXT_CLASS: Record<Tone, string> = {
  success: 'text-[var(--success)]',
  danger: 'text-[var(--danger)]',
  warning: 'text-[var(--warning)]',
  info: 'text-[var(--info)]',
  accent: 'text-[var(--accent)]',
  muted: 'text-[var(--text-secondary)]',
};
