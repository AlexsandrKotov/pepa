'use client';

import { useState, useEffect } from 'react';

export interface ExpressionCondition {
  field: string;
  operator: string;
  value: string;
}

interface Props {
  value: string;
  onChange: (expression: string) => void;
}

const fieldOptions = [
  { value: 'type_key', label: 'Type', category: 'basic' },
  { value: 'status', label: 'Status', category: 'basic' },
  { value: 'name', label: 'Name', category: 'basic' },
  { value: 'description', label: 'Description', category: 'basic' },
  { value: 'has_metadata', label: 'Has metadata field...', category: 'metadata' },
  { value: 'not_empty', label: 'Not empty field...', category: 'metadata' },
];

const operatorOptions = [
  { value: '==', label: 'equals' },
  { value: '!=', label: 'not equals' },
  { value: 'has_metadata', label: 'has metadata' },
  { value: 'not_empty', label: 'is not empty' },
];

const metadataFieldPresets = [
  'health_endpoint', 'owner', 'repository', 'replicas', 'image',
  'source', 'cluster', 'namespace', 'monitoring', 'documentation',
  'ci_cd', 'vault_secrets', 'resource_limits',
];

export default function ExpressionBuilder({ value, onChange }: Props) {
  const [conditions, setConditions] = useState<ExpressionCondition[]>([]);
  const [logicOp, setLogicOp] = useState<'&&' | '||'>('&&');
  const [mode, setMode] = useState<'visual' | 'raw'>('visual');

  // Parse existing expression on mount
  useEffect(() => {
    if (value && conditions.length === 0) {
      const parsed = parseExpression(value);
      if (parsed.conditions.length > 0) {
        setConditions(parsed.conditions);
        setLogicOp(parsed.logicOp);
      }
    }
  }, [value]);

  const addCondition = () => {
    setConditions([...conditions, { field: 'status', operator: '==', value: 'active' }]);
  };

  const removeCondition = (index: number) => {
    setConditions(conditions.filter((_, i) => i !== index));
  };

  const updateCondition = (index: number, updates: Partial<ExpressionCondition>) => {
    const updated = [...conditions];
    updated[index] = { ...updated[index], ...updates };
    setConditions(updated);
  };

  // Build expression string from conditions
  useEffect(() => {
    if (mode !== 'visual') return;
    const parts = conditions.map(c => {
      if (c.operator === 'has_metadata') {
        return `has_metadata.${c.value || 'field'}`;
      }
      if (c.operator === 'not_empty') {
        return `not_empty.${c.value || 'field'}`;
      }
      const val = c.value.includes(' ') ? `"${c.value}"` : c.value;
      return `${c.field} ${c.operator} ${val}`;
    });
    const expr = parts.join(` ${logicOp} `);
    onChange(expr);
  }, [conditions, logicOp, mode]);

  const handleRawChange = (raw: string) => {
    onChange(raw);
  };

  const toggleMode = () => {
    if (mode === 'visual') {
      setMode('raw');
    } else {
      // Try to parse raw expression back to visual
      const parsed = parseExpression(value);
      if (parsed.conditions.length > 0) {
        setConditions(parsed.conditions);
        setLogicOp(parsed.logicOp);
      }
      setMode('visual');
    }
  };

  return (
    <div className="space-y-3">
      {/* Mode toggle */}
      <div className="flex items-center justify-between">
        <span className="text-[11px] text-[var(--text-tertiary)]">
          {mode === 'visual' ? 'Visual builder' : 'Raw expression'}
        </span>
        <button type="button" onClick={toggleMode} className="text-[11px] text-[var(--accent)] hover:underline">
          Switch to {mode === 'visual' ? 'raw' : 'visual'}
        </button>
      </div>

      {mode === 'raw' ? (
        <input
          value={value}
          onChange={e => handleRawChange(e.target.value)}
          className="input font-mono text-[12px]"
          placeholder="e.g. metadata.health_endpoint == true && status == active"
        />
      ) : (
        <div className="space-y-2">
          {/* Logic operator toggle */}
          {conditions.length > 1 && (
            <div className="flex items-center gap-2">
              <span className="text-[11px] text-[var(--text-tertiary)]">Combine with:</span>
              <div className="flex gap-1">
                <button
                  type="button"
                  onClick={() => setLogicOp('&&')}
                  className={`px-2 py-0.5 rounded text-[11px] font-mono ${logicOp === '&&' ? 'bg-[var(--accent)]/10 text-[var(--accent)]' : 'bg-[var(--bg)] text-[var(--text-tertiary)]'}`}
                >
                  AND
                </button>
                <button
                  type="button"
                  onClick={() => setLogicOp('||')}
                  className={`px-2 py-0.5 rounded text-[11px] font-mono ${logicOp === '||' ? 'bg-[var(--accent)]/10 text-[var(--accent)]' : 'bg-[var(--bg)] text-[var(--text-tertiary)]'}`}
                >
                  OR
                </button>
              </div>
            </div>
          )}

          {/* Conditions */}
          {conditions.map((cond, i) => (
            <div key={i} className="flex gap-2 items-center">
              {/* Field selector */}
              <select
                value={cond.field}
                onChange={e => {
                  const field = e.target.value;
                  const updates: Partial<ExpressionCondition> = { field };
                  // Auto-set operator for special fields
                  if (field === 'has_metadata') updates.operator = 'has_metadata';
                  if (field === 'not_empty') updates.operator = 'not_empty';
                  updateCondition(i, updates);
                }}
                className="input text-[12px] flex-1"
              >
                <optgroup label="Fields">
                  {fieldOptions.filter(f => f.category === 'basic').map(f => (
                    <option key={f.value} value={f.value}>{f.label}</option>
                  ))}
                </optgroup>
                <optgroup label="Metadata checks">
                  {fieldOptions.filter(f => f.category === 'metadata').map(f => (
                    <option key={f.value} value={f.value}>{f.label}</option>
                  ))}
                </optgroup>
              </select>

              {/* Operator */}
              {(cond.field !== 'has_metadata' && cond.field !== 'not_empty') && (
                <select
                  value={cond.operator}
                  onChange={e => updateCondition(i, { operator: e.target.value })}
                  className="input text-[12px] w-[120px]"
                >
                  {operatorOptions.filter(o => o.value !== 'has_metadata' && o.value !== 'not_empty').map(o => (
                    <option key={o.value} value={o.value}>{o.label}</option>
                  ))}
                </select>
              )}

              {/* Value */}
              {(cond.field === 'has_metadata' || cond.field === 'not_empty' || cond.operator === 'has_metadata' || cond.operator === 'not_empty') ? (
                <select
                  value={cond.value}
                  onChange={e => updateCondition(i, { value: e.target.value })}
                  className="input text-[12px] flex-1"
                >
                  <option value="">Select field...</option>
                  {metadataFieldPresets.map(f => (
                    <option key={f} value={f}>{f}</option>
                  ))}
                </select>
              ) : cond.field === 'status' ? (
                <select
                  value={cond.value}
                  onChange={e => updateCondition(i, { value: e.target.value })}
                  className="input text-[12px] flex-1"
                >
                  <option value="active">active</option>
                  <option value="inactive">inactive</option>
                  <option value="deprecated">deprecated</option>
                </select>
              ) : cond.field === 'type_key' ? (
                <select
                  value={cond.value}
                  onChange={e => updateCondition(i, { value: e.target.value })}
                  className="input text-[12px] flex-1"
                >
                  <option value="service">service</option>
                  <option value="team">team</option>
                  <option value="environment">environment</option>
                  <option value="api_endpoint">api_endpoint</option>
                </select>
              ) : (
                <input
                  value={cond.value}
                  onChange={e => updateCondition(i, { value: e.target.value })}
                  className="input text-[12px] flex-1"
                  placeholder="value"
                />
              )}

              {/* Remove button */}
              <button type="button" onClick={() => removeCondition(i)} className="text-[var(--text-tertiary)] hover:text-red-500 text-[14px] px-1 shrink-0">✕</button>
            </div>
          ))}

          {/* Add condition */}
          <button type="button" onClick={addCondition} className="text-[11px] text-[var(--accent)] hover:underline">
            + Add condition
          </button>

          {/* Preview */}
          {value && (
            <div className="mt-2 p-2 rounded bg-[var(--bg)] border border-[var(--border-light)]">
              <span className="text-[10px] text-[var(--text-tertiary)] block mb-0.5">Expression:</span>
              <code className="text-[11px] font-mono text-[var(--text-secondary)] break-all">{value}</code>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ── Expression parser ────────────────────────────────────────

function parseExpression(expr: string): { conditions: ExpressionCondition[]; logicOp: '&&' | '||' } {
  if (!expr || !expr.trim()) return { conditions: [], logicOp: '&&' };

  // Detect logic operator
  const logicOp: '&&' | '||' = expr.includes(' || ') ? '||' : '&&';
  const separator = logicOp === '||' ? ' || ' : ' && ';

  const parts = expr.split(separator).map(p => p.trim()).filter(Boolean);
  const conditions: ExpressionCondition[] = [];

  for (const part of parts) {
    // has_metadata.field
    if (part.startsWith('has_metadata.')) {
      conditions.push({ field: 'has_metadata', operator: 'has_metadata', value: part.replace('has_metadata.', '') });
      continue;
    }
    // not_empty.field
    if (part.startsWith('not_empty.')) {
      conditions.push({ field: 'not_empty', operator: 'not_empty', value: part.replace('not_empty.', '') });
      continue;
    }
    // field == value or field != value
    const match = part.match(/^(\w+)\s*(==|!=)\s*(.+)$/);
    if (match) {
      conditions.push({ field: match[1], operator: match[2], value: match[3].replace(/"/g, '') });
      continue;
    }
  }

  return { conditions, logicOp };
}
