'use client';

import { useState, useRef, useEffect } from 'react';
import { glossary } from '@/lib/glossary';

interface ConceptHelpProps {
  term: string;
  label?: string;
}

/**
 * Small "?" control that opens a popover explaining a PEPA concept
 * in plain English (what it is, why it matters, example).
 */
export default function ConceptHelp({ term, label }: ConceptHelpProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const entry = glossary[term];

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', onClick);
    return () => document.removeEventListener('mousedown', onClick);
  }, [open]);

  if (!entry) return null;

  return (
    <div className="relative inline-block" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        className="inline-flex items-center gap-1 text-[11px] text-[#737373] hover:text-[#0066ff] transition-colors"
        aria-label={`What is ${label || term}?`}
      >
        <span className="w-4 h-4 rounded-full border border-[#d4d4d4] flex items-center justify-center text-[10px] font-medium">?</span>
        {label && <span>{label}</span>}
      </button>
      {open && (
        <div className="absolute z-50 left-0 top-6 w-80 bg-white border border-[#e5e5e5] rounded-xl shadow-lg p-4 text-left">
          <p className="text-[12px] font-semibold text-[#171717] mb-2 capitalize">{label || term}</p>
          <p className="text-[12px] text-[#525252] mb-2">{entry.what}</p>
          <p className="text-[12px] text-[#525252] mb-2">
            <span className="font-medium text-[#171717]">Why it matters: </span>
            {entry.why}
          </p>
          <p className="text-[11px] text-[#737373] bg-[#fafafa] rounded-lg p-2">
            <span className="font-medium text-[#525252]">Example: </span>
            {entry.example}
          </p>
        </div>
      )}
    </div>
  );
}
