import type { Tone } from '@/lib/filter-labels';

export interface FilterOption {
  value: string;
  label: string;
  /** Server-side facet count for this value; omitted while unknown. */
  count?: number;
  tone?: Tone;
  disabled?: boolean;
  /** Hide the option entirely when its count is 0. */
  hideWhenZero?: boolean;
}

export interface FilterGroup {
  key: string;
  label: string;
  options: FilterOption[];
  /** Comma-separated multi-select (OR semantics). Defaults to false. */
  multi?: boolean;
  /** Show a typeahead input inside the group. Defaults to true when > 7 options. */
  searchable?: boolean;
}
