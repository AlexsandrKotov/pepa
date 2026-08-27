'use client';

interface PaginationProps {
  page: number;
  perPage: number;
  total: number;
  onPageChange: (page: number) => void;
  onPerPageChange?: (perPage: number) => void;
}

export default function Pagination({ page, perPage, total, onPageChange, onPerPageChange }: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / perPage));
  const start = total === 0 ? 0 : (page - 1) * perPage + 1;
  const end = Math.min(page * perPage, total);

  // Build visible page numbers (show max 7 pages with ellipsis)
  const getPageNumbers = (): (number | '...')[] => {
    if (totalPages <= 7) return Array.from({ length: totalPages }, (_, i) => i + 1);
    const pages: (number | '...')[] = [];
    if (page <= 3) {
      pages.push(1, 2, 3, 4, '...', totalPages);
    } else if (page >= totalPages - 2) {
      pages.push(1, '...', totalPages - 3, totalPages - 2, totalPages - 1, totalPages);
    } else {
      pages.push(1, '...', page - 1, page, page + 1, '...', totalPages);
    }
    return pages;
  };

  if (total === 0) return null;

  return (
    <div className="flex items-center justify-between px-4 py-3 border-t border-[var(--border-light)]">
      {/* Info */}
      <div className="flex items-center gap-3">
        <span className="text-[12px] text-[var(--text-tertiary)]">
          {start}&ndash;{end} of {total}
        </span>
        {onPerPageChange && (
          <select
            value={perPage}
            onChange={e => onPerPageChange(Number(e.target.value))}
            className="text-[12px] text-[var(--text-secondary)] bg-transparent border border-[var(--border)] rounded px-1.5 py-0.5 focus:outline-none focus:border-[var(--accent)]"
          >
            {[10, 20, 50, 100].map(n => (
              <option key={n} value={n}>{n} / page</option>
            ))}
          </select>
        )}
      </div>

      {/* Page buttons */}
      <div className="flex items-center gap-1">
        <button
          onClick={() => onPageChange(page - 1)}
          disabled={page <= 1}
          className="px-2 py-1 text-[12px] rounded text-[var(--text-secondary)] hover:bg-[var(--border-light)] disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 19l-7-7 7-7" />
          </svg>
        </button>

        {getPageNumbers().map((p, idx) =>
          p === '...' ? (
            <span key={`ellipsis-${idx}`} className="px-2 py-1 text-[12px] text-[var(--text-tertiary)]">
              &hellip;
            </span>
          ) : (
            <button
              key={p}
              onClick={() => onPageChange(p)}
              className={`min-w-[28px] h-7 px-1.5 text-[12px] rounded transition-colors ${
                p === page
                  ? 'bg-[var(--accent)] text-white font-medium'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--border-light)]'
              }`}
            >
              {p}
            </button>
          ),
        )}

        <button
          onClick={() => onPageChange(page + 1)}
          disabled={page >= totalPages}
          className="px-2 py-1 text-[12px] rounded text-[var(--text-secondary)] hover:bg-[var(--border-light)] disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
          </svg>
        </button>
      </div>
    </div>
  );
}
