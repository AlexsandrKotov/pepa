'use client';

import type { ReactNode } from 'react';

interface FilterBarProps {
  /** <SearchInput /> slot. */
  search?: ReactNode;
  /** One or more <QuickFilter /> slot groups — the always-visible filters. */
  quick?: ReactNode;
  /** <FilterMenu /> for low-frequency facets. */
  menu?: ReactNode;
  /** Right-aligned extras: sort menu, view switch, density toggle. */
  actions?: ReactNode;
  /** <FilterChips /> row rendered underneath the control row. */
  chips?: ReactNode;
  /** Pins the bar to the top of the scroll container with a glass background. */
  sticky?: boolean;
  /** Request in flight — shows the hairline progress instead of blanking the list. */
  loading?: boolean;
  className?: string;
}

/**
 * One filter surface for the whole product: search, quick pills, facet menu,
 * right-aligned view actions and the chip row that states the applied filters.
 * Pages compose it through slots, so no config DSL stands between them and the primitives.
 *
 * Controls (search + menus + actions) share the first row; pills get a row of
 * their own, because a pill group is the unit that must never be split.
 */
export default function FilterBar({
  search,
  quick,
  menu,
  actions,
  chips,
  sticky = false,
  loading = false,
  className = '',
}: FilterBarProps) {
  const hasControls = Boolean(search || menu || actions);
  return (
    <div
      className={`filter-bar ${sticky ? 'filter-bar-sticky' : ''} ${className}`}
      data-loading={loading ? 'true' : 'false'}
      aria-busy={loading}
    >
      {hasControls && (
        <div className="filter-bar-row">
          {search}
          {menu}
          <div className="flex-1 min-w-0" />
          {actions}
        </div>
      )}
      {quick && <div className="filter-bar-row filter-bar-quick">{quick}</div>}
      {chips}
    </div>
  );
}
