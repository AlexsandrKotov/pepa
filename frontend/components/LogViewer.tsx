'use client';

import { useState, useRef, useEffect, useCallback, useMemo } from 'react';

interface LogViewerProps {
  logs: string;
  loading: boolean;
  onRefresh: () => void;
  onLineCountChange?: (lines: number) => void;
  currentLines?: number;
  title?: string;
}

const LINE_COUNTS = [100, 200, 500, 1000];

/** Colorise log lines by level keywords (ERROR / WARN / INFO). */
function colourLine(line: string): string | null {
  if (/\b(?:error|fatal|panic|crit|fail(?:ed)?)\b/i.test(line)) return 'text-red-400';
  if (/\b(?:warn(?:ing)?)\b/i.test(line)) return 'text-yellow-400';
  if (/\b(?:info)\b/i.test(line)) return 'text-blue-400';
  if (/\b(?:debug|trace)\b/i.test(line)) return 'text-gray-500';
  return null;
}

export default function LogViewer({
  logs,
  loading,
  onRefresh,
  onLineCountChange,
  currentLines = 200,
  title = 'Logs',
}: LogViewerProps) {
  const [expanded, setExpanded] = useState(false);
  const [search, setSearch] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const [copied, setCopied] = useState(false);
  const [showLineCounts, setShowLineCounts] = useState(false);
  const preRef = useRef<HTMLPreElement>(null);

  // Auto-scroll to bottom when logs change
  useEffect(() => {
    if (autoScroll && preRef.current) {
      preRef.current.scrollTop = preRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // ESC to exit expanded mode
  useEffect(() => {
    if (!expanded) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setExpanded(false);
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [expanded]);

  // Lock body scroll when expanded
  useEffect(() => {
    if (expanded) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => { document.body.style.overflow = ''; };
  }, [expanded]);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(logs);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // fallback
      const ta = document.createElement('textarea');
      ta.value = logs;
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [logs]);

  // Filter lines by search
  const filteredLines = useMemo(() => {
    const raw = logs || '';
    const lines = raw.split('\n');
    if (!search.trim()) return lines;
    const q = search.toLowerCase();
    return lines.filter(l => l.toLowerCase().includes(q));
  }, [logs, search]);

  const lineCount = filteredLines.length;

  // Toolbar shared classes
  const btnClass = 'p-1.5 rounded-md transition-colors text-[var(--text-tertiary)] hover:text-[var(--text-primary)] hover:bg-white/10';
  const activeBtnClass = 'p-1.5 rounded-md transition-colors text-[var(--accent)] bg-[var(--accent)]/15 hover:bg-[var(--accent)]/25';

  const toolbar = (
    <div className="flex items-center gap-1.5 flex-wrap">
      {/* Search */}
      <div className="relative">
        <svg className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-gray-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
        </svg>
        <input
          type="text"
          placeholder="Filter logs..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="bg-white/5 border border-white/10 rounded-md pl-7 pr-2 py-1 text-[11px] text-gray-200 placeholder-gray-500 outline-none focus:border-[var(--accent)]/40 w-40"
        />
        {search && (
          <span className="absolute right-1.5 top-1/2 -translate-y-1/2 text-[9px] text-gray-500">
            {lineCount} matches
          </span>
        )}
      </div>

      {/* Line count selector */}
      <div className="relative">
        <button
          onClick={() => setShowLineCounts(!showLineCounts)}
          className={btnClass}
          title="Lines"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h10" />
          </svg>
        </button>
        {showLineCounts && (
          <div className="absolute top-full mt-1 left-0 bg-[#252535] border border-white/10 rounded-lg shadow-xl z-50 py-1 min-w-[100px]">
            {LINE_COUNTS.map(n => (
              <button
                key={n}
                onClick={() => { onLineCountChange?.(n); setShowLineCounts(false); }}
                className={`block w-full text-left px-3 py-1 text-[11px] transition-colors ${n === currentLines ? 'text-[var(--accent)] bg-[var(--accent)]/10' : 'text-gray-300 hover:bg-white/5'}`}
              >
                {n} lines
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Auto-scroll toggle */}
      <button
        onClick={() => setAutoScroll(!autoScroll)}
        className={autoScroll ? activeBtnClass : btnClass}
        title={autoScroll ? 'Auto-scroll ON' : 'Auto-scroll OFF'}
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
        </svg>
      </button>

      {/* Copy */}
      <button onClick={handleCopy} className={btnClass} title={copied ? 'Copied!' : 'Copy to clipboard'}>
        {copied ? (
          <svg className="w-3.5 h-3.5 text-emerald-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        ) : (
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
        )}
      </button>

      {/* Refresh */}
      <button onClick={onRefresh} className={btnClass} title="Refresh" disabled={loading}>
        <svg className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
        </svg>
      </button>

      {/* Expand / Collapse */}
      <button
        onClick={() => setExpanded(!expanded)}
        className={btnClass}
        title={expanded ? 'Collapse (Esc)' : 'Expand to fullscreen'}
      >
        {expanded ? (
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 9V4.5M9 9H4.5M9 9L3.75 3.75M9 15v4.5M9 15H4.5M9 15l-5.25 5.25M15 9h4.5M15 9V4.5M15 9l5.25-5.25M15 15h4.5M15 15v4.5m0-4.5l5.25 5.25" />
          </svg>
        ) : (
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 3.75v4.5m0-4.5h4.5m-4.5 0L9 9M3.75 20.25v-4.5m0 4.5h4.5m-4.5 0L9 15M20.25 3.75h-4.5m4.5 0v4.5m0-4.5L15 9m5.25 11.25h-4.5m4.5 0v-4.5m0 4.5L15 15" />
          </svg>
        )}
      </button>
    </div>
  );

  // Render log content with level-based colouring
  const logContent = (
    <pre
      ref={preRef}
      className="bg-[#1e1e2e] text-[#cdd6f4] text-[11px] font-mono overflow-auto whitespace-pre-wrap leading-relaxed select-text"
      style={{ tabSize: 2 }}
    >
      {filteredLines.map((line, i) => {
        const cls = colourLine(line);
        return (
          <div key={i} className={cls || ''}>
            {line}
          </div>
        );
      })}
      {filteredLines.length === 0 && (
        <span className="text-gray-500 italic">
          {search ? 'No lines match your filter' : 'No logs available'}
        </span>
      )}
    </pre>
  );

  // Expanded (fullscreen) mode
  if (expanded) {
    return (
      <div className="fixed inset-0 z-[200] flex flex-col bg-[#1e1e2e]">
        {/* Header bar */}
        <div className="flex items-center justify-between px-4 py-2 bg-[#181828] border-b border-white/10 shrink-0">
          <div className="flex items-center gap-3">
            <h3 className="text-[13px] font-semibold text-gray-200">{title}</h3>
            <span className="text-[10px] text-gray-500 font-mono">{lineCount} lines</span>
          </div>
          {toolbar}
        </div>
        {/* Log content fills remaining space */}
        <div className="flex-1 overflow-hidden">
          {logContent}
        </div>
      </div>
    );
  }

  // Inline mode
  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-2">
          <h3 className="text-[12px] font-semibold text-[var(--text-secondary)]">{title}</h3>
          <span className="text-[10px] text-[var(--text-tertiary)] font-mono">{lineCount} lines</span>
        </div>
        {toolbar}
      </div>
      {loading ? (
        <div className="bg-[#1e1e2e] rounded-lg p-6 flex items-center justify-center">
          <svg className="w-5 h-5 animate-spin text-[var(--accent)]" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
        </div>
      ) : (
        <div className="rounded-lg overflow-hidden border border-white/5 max-h-[500px]">
          {logContent}
        </div>
      )}
    </div>
  );
}
