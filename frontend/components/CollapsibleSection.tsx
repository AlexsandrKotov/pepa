'use client';

import { useState, useEffect, type ReactNode } from 'react';

interface CollapsibleSectionProps {
  id: string;
  title: string;
  defaultExpanded?: boolean;
  action?: ReactNode;
  children: ReactNode;
}

export default function CollapsibleSection({ id, title, defaultExpanded = true, action, children }: CollapsibleSectionProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  useEffect(() => {
    const saved = localStorage.getItem(`dashboard-${id}`);
    if (saved !== null) setExpanded(saved === 'expanded');
  }, [id]);

  const toggle = () => {
    const next = !expanded;
    setExpanded(next);
    localStorage.setItem(`dashboard-${id}`, next ? 'expanded' : 'collapsed');
  };

  return (
    <div className="card">
      <div className="card-header">
        <button
          onClick={toggle}
          className="flex items-center gap-2 text-[13px] font-medium text-[#171717] hover:text-[#0066ff] transition-colors"
        >
          <svg
            className={`w-3.5 h-3.5 transition-transform ${expanded ? 'rotate-0' : '-rotate-90'}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
          {title}
        </button>
        {action && expanded && <div>{action}</div>}
      </div>
      {expanded && <div>{children}</div>}
    </div>
  );
}
