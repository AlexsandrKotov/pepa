'use client';

import { useState, useMemo } from 'react';

// ANSI color code to CSS color mapping
const ANSI_COLORS: Record<number, string> = {
  30: 'var(--text-secondary)', // black
  31: '#e54',                  // red
  32: '#4c4',                  // green
  33: '#cc3',                  // yellow
  34: '#48f',                  // blue
  35: '#c4c',                  // magenta
  36: '#4cc',                  // cyan
  37: 'var(--text-primary)',   // white
  90: 'var(--text-tertiary)',  // bright black (gray)
  91: '#f66',                  // bright red
  92: '#6f6',                  // bright green
  93: '#ff6',                  // bright yellow
  94: '#69f',                  // bright blue
  95: '#f6f',                  // bright magenta
  96: '#6ff',                  // bright cyan
  97: '#fff',                  // bright white
};

const ANSI_BG_COLORS: Record<number, string> = {
  40: '#333', 41: '#600', 42: '#060', 43: '#660',
  44: '#006', 45: '#606', 46: '#066', 47: '#666',
};

interface AnsiSegment {
  text: string;
  color?: string;
  bg?: string;
  bold?: boolean;
}

function parseAnsi(input: string): AnsiSegment[] {
  const segments: AnsiSegment[] = [];
  // Match ANSI escape sequences: ESC[<codes>m
  const re = /\x1b\[([0-9;]*)m/g;
  let lastIndex = 0;
  let currentColor: string | undefined;
  let currentBg: string | undefined;
  let currentBold = false;
  let match: RegExpExecArray | null;

  while ((match = re.exec(input)) !== null) {
    // Push text before this escape sequence
    if (match.index > lastIndex) {
      segments.push({ text: input.slice(lastIndex, match.index), color: currentColor, bg: currentBg, bold: currentBold });
    }
    lastIndex = match.index + match[0].length;

    // Parse codes
    const codes = match[1].split(';').map(Number);
    for (const code of codes) {
      if (code === 0) { currentColor = undefined; currentBg = undefined; currentBold = false; }
      else if (code === 1) { currentBold = true; }
      else if (code === 22) { currentBold = false; }
      else if (ANSI_COLORS[code]) { currentColor = ANSI_COLORS[code]; }
      else if (ANSI_BG_COLORS[code]) { currentBg = ANSI_BG_COLORS[code]; }
      else if (code === 39) { currentColor = undefined; }
      else if (code === 49) { currentBg = undefined; }
    }
  }
  // Push remaining text
  if (lastIndex < input.length) {
    segments.push({ text: input.slice(lastIndex), color: currentColor, bg: currentBg, bold: currentBold });
  }
  return segments;
}

// Escape HTML entities to prevent XSS before applying highlight spans
function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// IaC output pattern-based highlighting (for Terraform/OpenTofu which doesn't emit ANSI in non-TTY)
function highlightIac(text: string): string {
  const safe = escapeHtml(text);
  return safe
    .replace(/^(\s*)# (.+)$/, '$1<span style="color:var(--text-tertiary)"># $2</span>')
    .replace(/^(\s*)(Terraform|OpenTofu)( used the selected providers.*)$/, '$1<span style="color:#48f;font-weight:600">$2</span>$3')
    .replace(/^(Plan: .+)$/, '<span style="color:#cc3;font-weight:600">$1</span>')
    .replace(/^(Apply complete!.+)$/, '<span style="color:#4c4;font-weight:600">$1</span>')
    .replace(/^(Error:.+)$/, '<span style="color:#e54;font-weight:600">$1</span>')
    .replace(/^(Warning:.+)$/, '<span style="color:#cc3;font-weight:600">$1</span>')
    .replace(/(will be created)/g, '<span style="color:#4c4">$1</span>')
    .replace(/(will be destroyed)/g, '<span style="color:#e54">$1</span>')
    .replace(/(will be updated|must be replaced)/g, '<span style="color:#cc3">$1</span>')
    .replace(/(\+\s.+)$/gm, '<span style="color:#4c4">$1</span>')
    .replace(/(-\s.+)$/gm, '<span style="color:#e54">$1</span>')
    .replace(/(~\s.+)$/gm, '<span style="color:#cc3">$1</span>');
}

// Ansible output pattern-based highlighting (fallback for lines without ANSI)
function highlightAnsible(text: string): string {
  const safe = escapeHtml(text);
  return safe
    .replace(/^(PLAY \[.+?\] \*+)$/, '<span style="color:#48f;font-weight:700">$1</span>')
    .replace(/^(TASK \[.+?\] \*+)$/, '<span style="color:#cc3;font-weight:600">$1</span>')
    .replace(/^(ok: \[.+?\])$/, '<span style="color:#4c4">$1</span>')
    .replace(/^(changed: \[.+?\])$/, '<span style="color:#cc3">$1</span>')
    .replace(/^(failed: \[.+?\])$/, '<span style="color:#e54;font-weight:600">$1</span>')
    .replace(/^(skipping: \[.+?\])$/, '<span style="color:var(--text-tertiary)">$1</span>')
    .replace(/^(PLAY RECAP \*+)$/, '<span style="color:#48f;font-weight:700">$1</span>')
    .replace(/(ok=\d+)/g, '<span style="color:#4c4">$1</span>')
    .replace(/(changed=\d+)/g, '<span style="color:#cc3">$1</span>')
    .replace(/(failed=\d+)/g, '<span style="color:#e54">$1</span>')
    .replace(/(unreachable=\d+)/g, '<span style="color:#e54;font-weight:600">$1</span>');
}

function hasAnsiCodes(text: string): boolean {
  return /\x1b\[[\d;]*m/.test(text);
}

interface AnsiOutputProps {
  text: string;
  mode?: 'auto' | 'ansi' | 'iac' | 'ansible';
  maxHeight?: string;
  className?: string;
}

export default function AnsiOutput({ text, mode = 'auto', maxHeight = '320px', className = '' }: AnsiOutputProps) {
  const [expanded, setExpanded] = useState(false);

  const rendered = useMemo(() => {
    // Always try ANSI parsing first — if real ANSI codes are present, render them as colors.
    // This works regardless of the mode prop (auto/ansible/iac).
    if (hasAnsiCodes(text)) {
      const segments = parseAnsi(text);
      if (segments.length > 0) {
        return segments.map((seg, i) => (
          <span key={i} style={{
            color: seg.color,
            backgroundColor: seg.bg,
            fontWeight: seg.bold ? 600 : undefined,
          }}>{seg.text}</span>
        ));
      }
    }

    // No ANSI codes found — fall back to pattern-based highlighting
    const effectiveMode = mode === 'auto'
      ? (text.includes('to add') || text.includes('Terraform') || text.includes('OpenTofu') ? 'iac' : 'ansible')
      : mode;
    const lines = text.split('\n');
    const highlightFn = effectiveMode === 'iac' ? highlightIac : highlightAnsible;
    return lines.map((line, i) => (
      <span key={i} dangerouslySetInnerHTML={{ __html: highlightFn(line) + '\n' }} />
    ));
  }, [text, mode]);

  const isLong = text.split('\n').length > 20;

  return (
    <div className={`relative ${className}`}>
      <pre
        className="text-[11px] font-mono bg-[var(--surface)] rounded-lg p-3 overflow-auto whitespace-pre-wrap leading-relaxed"
        style={{ maxHeight: expanded ? '70vh' : maxHeight }}
      >
        {rendered}
      </pre>
      {isLong && (
        <button
          onClick={() => setExpanded(e => !e)}
          className="absolute bottom-2 right-2 px-2 py-0.5 text-[10px] font-medium rounded bg-[var(--bg)] border border-[var(--border)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--border-light)] transition-colors"
        >
          {expanded ? 'Collapse' : 'Expand'}
        </button>
      )}
    </div>
  );
}
