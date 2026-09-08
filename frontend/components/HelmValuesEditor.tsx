'use client';

import { useState } from 'react';

interface HelmValuesEditorProps {
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  depth?: number;
}

/** Recursive visual editor for Helm values.yaml structures. */
export default function HelmValuesEditor({ values, onChange, depth = 0 }: HelmValuesEditorProps) {
  const entries = Object.entries(values);

  if (entries.length === 0) {
    return (
      <p className="text-[12px] text-[var(--text-tertiary)] text-center py-4">
        No values defined
      </p>
    );
  }

  return (
    <div className={depth > 0 ? 'ml-3 border-l border-[var(--border-light)] pl-3 space-y-2' : 'space-y-2'}>
      {entries.map(([key, value]) => (
        <ValueRow
          key={key}
          path={key}
          value={value}
          onUpdate={(newVal) => {
            onChange({ ...values, [key]: newVal });
          }}
          onDelete={() => {
            const next = { ...values };
            delete next[key];
            onChange(next);
          }}
          depth={depth}
        />
      ))}
    </div>
  );
}

interface ValueRowProps {
  path: string;
  value: unknown;
  onUpdate: (val: unknown) => void;
  onDelete: () => void;
  depth: number;
}

function ValueRow({ path, value, onUpdate, onDelete, depth }: ValueRowProps) {
  const [collapsed, setCollapsed] = useState(depth > 1);

  // Object: collapsible section with nested fields
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    const obj = value as Record<string, unknown>;
    const keys = Object.keys(obj);
    return (
      <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)]">
        <button
          type="button"
          onClick={() => setCollapsed(!collapsed)}
          className="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-[var(--bg-hover)] transition-colors rounded-lg"
        >
          <div className="flex items-center gap-2 min-w-0">
            <span className={`text-[10px] text-[var(--text-tertiary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}>
              {'\u25B6'}
            </span>
            <span className="text-[12px] font-medium text-[var(--text-primary)] truncate">{path}</span>
            <span className="text-[10px] text-[var(--text-tertiary)] shrink-0">
              {'{'}{keys.length} keys{'}'}
            </span>
          </div>
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onDelete(); }}
            className="text-[10px] text-red-400 hover:text-red-600 shrink-0 ml-2"
            title="Remove"
          >
            {'\u2715'}
          </button>
        </button>
        {!collapsed && (
          <div className="px-3 pb-3">
            <HelmValuesEditor values={obj} onChange={onUpdate} depth={depth + 1} />
          </div>
        )}
      </div>
    );
  }

  // Array: list with add/remove
  if (Array.isArray(value)) {
    return (
      <div className="rounded-lg border border-[var(--border)] bg-[var(--bg)]">
        <button
          type="button"
          onClick={() => setCollapsed(!collapsed)}
          className="w-full flex items-center justify-between px-3 py-2 text-left hover:bg-[var(--bg-hover)] transition-colors rounded-lg"
        >
          <div className="flex items-center gap-2 min-w-0">
            <span className={`text-[10px] text-[var(--text-tertiary)] transition-transform ${collapsed ? '' : 'rotate-90'}`}>
              {'\u25B6'}
            </span>
            <span className="text-[12px] font-medium text-[var(--text-primary)] truncate">{path}</span>
            <span className="text-[10px] text-[var(--text-tertiary)] shrink-0">
              [{value.length} items]
            </span>
          </div>
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); onDelete(); }}
            className="text-[10px] text-red-400 hover:text-red-600 shrink-0 ml-2"
            title="Remove"
          >
            {'\u2715'}
          </button>
        </button>
        {!collapsed && (
          <div className="px-3 pb-3 space-y-1">
            {value.map((item, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <span className="text-[10px] text-[var(--text-tertiary)] w-5 text-right shrink-0">{idx}</span>
                <ArrayItemEditor
                  value={item}
                  onUpdate={(newItem) => {
                    const next = [...value];
                    next[idx] = newItem;
                    onUpdate(next);
                  }}
                  onDelete={() => {
                    const next = value.filter((_, i) => i !== idx);
                    onUpdate(next);
                  }}
                />
              </div>
            ))}
            <button
              type="button"
              onClick={() => onUpdate([...value, ''])}
              className="text-[11px] text-[var(--accent)] hover:underline mt-1"
            >
              + Add item
            </button>
          </div>
        )}
      </div>
    );
  }

  // Boolean: toggle
  if (typeof value === 'boolean') {
    return (
      <div className="flex items-center gap-2 px-1 group">
        <label className="text-[12px] text-[var(--text-secondary)] min-w-[120px] truncate" title={path}>
          {path}
        </label>
        <button
          type="button"
          onClick={() => onUpdate(!value)}
          className={`w-8 h-4 rounded-full transition-colors relative ${value ? 'bg-[var(--accent)]' : 'bg-[var(--border)]'}`}
        >
          <span className={`absolute top-0.5 w-3 h-3 rounded-full bg-white transition-transform ${value ? 'left-[18px]' : 'left-0.5'}`} />
        </button>
        <span className="text-[10px] text-[var(--text-tertiary)]">{value ? 'true' : 'false'}</span>
        <button
          type="button"
          onClick={onDelete}
          className="text-[10px] text-red-400 hover:text-red-600 opacity-0 group-hover:opacity-100 ml-auto"
          title="Remove"
        >
          {'\u2715'}
        </button>
      </div>
    );
  }

  // Number: number input
  if (typeof value === 'number') {
    return (
      <div className="flex items-center gap-2 px-1 group">
        <label className="text-[12px] text-[var(--text-secondary)] min-w-[120px] truncate" title={path}>
          {path}
        </label>
        <input
          type="number"
          value={value}
          onChange={e => onUpdate(Number(e.target.value))}
          className="input text-[12px] w-28 py-1"
        />
        <button
          type="button"
          onClick={onDelete}
          className="text-[10px] text-red-400 hover:text-red-600 opacity-0 group-hover:opacity-100 ml-auto"
          title="Remove"
        >
          {'\u2715'}
        </button>
      </div>
    );
  }

  // String (or null): text input
  return (
    <div className="flex items-center gap-2 px-1 group">
      <label className="text-[12px] text-[var(--text-secondary)] min-w-[120px] truncate" title={path}>
        {path}
      </label>
      <input
        type="text"
        value={value === null ? '' : String(value)}
        onChange={e => onUpdate(e.target.value)}
        className="input text-[12px] flex-1 py-1"
        placeholder={value === null ? '(empty)' : ''}
      />
      <button
        type="button"
        onClick={onDelete}
        className="text-[10px] text-red-400 hover:text-red-600 opacity-0 group-hover:opacity-100 shrink-0"
        title="Remove"
      >
        {'\u2715'}
      </button>
    </div>
  );
}

interface ArrayItemEditorProps {
  value: unknown;
  onUpdate: (val: unknown) => void;
  onDelete: () => void;
}

function ArrayItemEditor({ value, onUpdate, onDelete }: ArrayItemEditorProps) {
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    const obj = value as Record<string, unknown>;
    return (
      <div className="flex-1 rounded border border-[var(--border-light)] p-2 space-y-1">
        {Object.entries(obj).map(([k, v]) => (
          <div key={k} className="flex items-center gap-2">
            <span className="text-[10px] text-[var(--text-tertiary)] w-20 truncate">{k}</span>
            <input
              type="text"
              value={v === null ? '' : typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean' ? String(v) : JSON.stringify(v)}
              onChange={e => {
                let parsed: unknown = e.target.value;
                if (typeof v === 'number') parsed = Number(e.target.value);
                else if (typeof v === 'boolean') parsed = e.target.value === 'true';
                onUpdate({ ...obj, [k]: parsed });
              }}
              className="input text-[11px] flex-1 py-0.5"
            />
          </div>
        ))}
        <button type="button" onClick={onDelete} className="text-[10px] text-red-400 hover:text-red-600">
          Remove
        </button>
      </div>
    );
  }

  // Primitive array item
  return (
    <div className="flex items-center gap-2 flex-1">
      <input
        type="text"
        value={value === null ? '' : typeof value === 'string' || typeof value === 'number' ? String(value) : JSON.stringify(value)}
        onChange={e => {
          if (typeof value === 'number') onUpdate(Number(e.target.value));
          else onUpdate(e.target.value);
        }}
        className="input text-[11px] flex-1 py-0.5"
      />
      <button type="button" onClick={onDelete} className="text-[10px] text-red-400 hover:text-red-600">
        {'\u2715'}
      </button>
    </div>
  );
}

/** Simple YAML serializer for converting edited values back to YAML string. */
export function toYaml(obj: unknown, indent = 0): string {
  if (obj === null || obj === undefined) return '""';
  if (typeof obj === 'string') {
    // Quote strings that could be misinterpreted
    if (obj === '' || obj === 'true' || obj === 'false' || obj === 'null' || /^\d/.test(obj) || /[:#{}[\],&*?|>!%@`]/.test(obj)) {
      return `"${obj.replace(/"/g, '\\"')}"`;
    }
    return obj;
  }
  if (typeof obj === 'number' || typeof obj === 'boolean') return String(obj);

  const prefix = '  '.repeat(indent);

  if (Array.isArray(obj)) {
    if (obj.length === 0) return '[]';
    return obj.map(item => {
      if (item !== null && typeof item === 'object' && !Array.isArray(item)) {
        const entries = Object.entries(item as Record<string, unknown>);
        if (entries.length === 0) return `${prefix}- {}`;
        const first = entries[0];
        const rest = entries.slice(1);
        const firstLine = `${prefix}- ${first[0]}: ${toYaml(first[1], indent + 1)}`;
        const restLines = rest.map(([k, v]) => `${prefix}  ${k}: ${toYaml(v, indent + 1)}`);
        return [firstLine, ...restLines].join('\n');
      }
      return `${prefix}- ${toYaml(item, indent + 1)}`;
    }).join('\n');
  }

  if (typeof obj === 'object') {
    const entries = Object.entries(obj as Record<string, unknown>);
    if (entries.length === 0) return '{}';
    return entries.map(([k, v]) => {
      if (v !== null && typeof v === 'object') {
        return `${prefix}${k}:\n${toYaml(v, indent + 1)}`;
      }
      return `${prefix}${k}: ${toYaml(v, indent)}`;
    }).join('\n');
  }

  return String(obj);
}

/** Simple YAML parser for converting YAML string back to object. */
export function fromYaml(yamlStr: string): Record<string, unknown> | null {
  try {
    // Use a minimal YAML parser that handles common cases
    const result: Record<string, unknown> = {};
    const lines = yamlStr.split('\n');
    const stack: { indent: number; obj: Record<string, unknown>; key?: string }[] = [];
    stack.push({ indent: -1, obj: result });

    for (const rawLine of lines) {
      const line = rawLine.replace(/\r$/, '');
      if (line.trim() === '' || line.trim().startsWith('#')) continue;

      const indent = line.search(/\S/);
      const trimmed = line.trim();

      // Pop stack to find parent
      while (stack.length > 1 && stack[stack.length - 1].indent >= indent) {
        stack.pop();
      }

      const parent = stack[stack.length - 1];

      if (trimmed.startsWith('- ')) {
        // Array item
        const arrayKey = parent.key;
        if (arrayKey && !Array.isArray(parent.obj[arrayKey])) {
          parent.obj[arrayKey] = [];
        }
        if (arrayKey) {
          const itemVal = trimmed.slice(2).trim();
          const arr = parent.obj[arrayKey] as unknown[];
          if (itemVal.includes(': ')) {
            // Object in array
            const obj: Record<string, unknown> = {};
            const [k, ...rest] = itemVal.split(': ');
            obj[k] = parseScalar(rest.join(': '));
            arr.push(obj);
            stack.push({ indent, obj, key: k });
          } else {
            arr.push(parseScalar(itemVal));
          }
        }
        continue;
      }

      const colonIdx = trimmed.indexOf(':');
      if (colonIdx === -1) continue;

      const key = trimmed.slice(0, colonIdx).trim();
      const valStr = trimmed.slice(colonIdx + 1).trim();

      if (valStr === '') {
        // Nested object or array
        parent.obj[key] = {};
        stack.push({ indent, obj: parent.obj, key });
      } else {
        parent.obj[key] = parseScalar(valStr);
      }
    }

    return Object.keys(result).length > 0 ? result : null;
  } catch {
    return null;
  }
}

function parseScalar(val: string): unknown {
  if (val === 'true') return true;
  if (val === 'false') return false;
  if (val === 'null' || val === '~') return null;
  if (/^-?\d+$/.test(val)) return parseInt(val, 10);
  if (/^-?\d+\.\d+$/.test(val)) return parseFloat(val);
  // Strip quotes
  if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
    return val.slice(1, -1);
  }
  return val;
}
