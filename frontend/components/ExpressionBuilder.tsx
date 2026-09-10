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
  { value: 'sync_status', label: 'Sync Status', category: 'basic' },
  { value: 'plugin_name', label: 'Plugin Name', category: 'basic' },
  { value: 'has_metadata', label: 'Has metadata field...', category: 'metadata' },
  { value: 'not_empty', label: 'Not empty field...', category: 'metadata' },
];

const operatorOptions = [
  { value: '==', label: 'equals' },
  { value: '!=', label: 'not equals' },
  { value: '>=', label: '≥ greater or equal' },
  { value: '<=', label: '≤ less or equal' },
  { value: '>', label: '> greater' },
  { value: '<', label: '< less' },
  { value: 'contains', label: 'contains' },
  { value: 'starts_with', label: 'starts with' },
  { value: 'has_metadata', label: 'has metadata' },
  { value: 'not_empty', label: 'is not empty' },
];

const metadataFieldPresets = [
  'health_endpoint', 'owner', 'repository', 'replicas', 'image',
  'source', 'cluster', 'namespace', 'monitoring', 'documentation',
  'ci_cd', 'vault_secrets', 'resource_limits', 'tags', 'version',
  'environment',
];

const quickExamples = [
  { label: 'Active service', expr: 'status == active && type_key == service' },
  { label: 'At least 2 replicas', expr: 'metadata.replicas >= 2' },
  { label: 'Has owner & repo', expr: 'has_metadata.owner && has_metadata.repository' },
  { label: 'Prod or staging', expr: '(metadata.environment == production || metadata.environment == staging)' },
  { label: 'Image from registry', expr: 'metadata.image starts_with "registry.example.com/"' },
  { label: 'Vault + monitoring', expr: 'has_metadata.vault_secrets && has_metadata.monitoring' },
];

export default function ExpressionBuilder({ value, onChange }: Props) {
  const [conditions, setConditions] = useState<ExpressionCondition[]>([]);
  const [logicOp, setLogicOp] = useState<'&&' | '||'>('&&');
  const [mode, setMode] = useState<'visual' | 'raw'>('visual');
  const [showHelp, setShowHelp] = useState(false);

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
      if (c.operator === 'contains' || c.operator === 'starts_with') {
        const val = c.value || 'value';
        return `${c.field} ${c.operator} "${val}"`;
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
      // Parentheses expressions are raw-only — stay in raw mode
      if (value.includes('(') || value.includes(')')) {
        setMode('raw');
        return;
      }
      // Try to parse raw expression back to visual
      const parsed = parseExpression(value);
      if (parsed.conditions.length > 0) {
        setConditions(parsed.conditions);
        setLogicOp(parsed.logicOp);
      }
      setMode('visual');
    }
  };

  // Operators available for basic fields (not has_metadata/not_empty)
  const basicOperators = operatorOptions.filter(o => o.value !== 'has_metadata' && o.value !== 'not_empty');

  // Check if operator is numeric-only
  const isNumericOp = (op: string) => ['>=', '<=', '>', '<'].includes(op);
  // Check if operator is string-only
  const isStringOp = (op: string) => ['contains', 'starts_with'].includes(op);

  return (
    <div className="space-y-3">
      {/* Mode toggle + Help */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <span className="text-[11px] text-[var(--text-tertiary)]">
            {mode === 'visual' ? 'Visual builder' : 'Raw expression'}
          </span>
          <button
            type="button"
            onClick={() => setShowHelp(!showHelp)}
            className={`text-[11px] px-1.5 py-0.5 rounded transition-colors ${showHelp ? 'bg-[var(--accent)]/10 text-[var(--accent)]' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'}`}
          >
            {showHelp ? '✕ close help' : '? reference'}
          </button>
        </div>
        <button type="button" onClick={toggleMode} className="text-[11px] text-[var(--accent)] hover:underline">
          Switch to {mode === 'visual' ? 'raw' : 'visual'}
        </button>
      </div>

      {/* Help / Reference panel */}
      {showHelp && (
        <div className="p-3 rounded-lg border border-[var(--border-light)] bg-[var(--bg)] space-y-3">
          <p className="text-[11px] font-medium text-[var(--text-primary)]">Available Fields</p>
          <div className="grid grid-cols-2 gap-x-4 gap-y-1">
            <div className="text-[10px] text-[var(--text-tertiary)]">
              <span className="font-mono text-[var(--text-secondary)]">type_key</span> — entity type (service, team, ...)
            </div>
            <div className="text-[10px] text-[var(--text-tertiary)]">
              <span className="font-mono text-[var(--text-secondary)]">status</span> — active, inactive, deprecated
            </div>
            <div className="text-[10px] text-[var(--text-tertiary)]">
              <span className="font-mono text-[var(--text-secondary)]">name</span> — entity name
            </div>
            <div className="text-[10px] text-[var(--text-tertiary)]">
              <span className="font-mono text-[var(--text-secondary)]">description</span> — entity description
            </div>
            <div className="text-[10px] text-[var(--text-tertiary)]">
              <span className="font-mono text-[var(--text-secondary)]">sync_status</span> — synced, pending, error
            </div>
            <div className="text-[10px] text-[var(--text-tertiary)]">
              <span className="font-mono text-[var(--text-secondary)]">plugin_name</span> — source plugin
            </div>
            <div className="text-[10px] text-[var(--text-tertiary)] col-span-2">
              <span className="font-mono text-[var(--text-secondary)]">metadata.*</span> — any metadata field (e.g. <span className="font-mono">replicas</span>, <span className="font-mono">owner</span>, <span className="font-mono">image</span>)
            </div>
          </div>

          <p className="text-[11px] font-medium text-[var(--text-primary)]">Operators</p>
          <div className="grid grid-cols-2 gap-x-4 gap-y-1">
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">==</span> / <span className="font-mono text-[var(--text-secondary)]">!=</span> — equals / not equals</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">&gt;=</span> / <span className="font-mono text-[var(--text-secondary)]">&lt;=</span> — numeric comparison</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">&gt;</span> / <span className="font-mono text-[var(--text-secondary)]">&lt;</span> — numeric comparison</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">contains</span> — substring match</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">starts_with</span> — prefix match</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">has_metadata.X</span> — field exists</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">not_empty.X</span> — non-empty check</div>
            <div className="text-[10px] text-[var(--text-tertiary)]"><span className="font-mono text-[var(--text-secondary)]">&&</span> / <span className="font-mono text-[var(--text-secondary)]">||</span> — AND / OR</div>
          </div>

          <p className="text-[11px] font-medium text-[var(--text-primary)]">Quick Examples</p>
          <div className="space-y-1">
            {quickExamples.map((ex, i) => (
              <button
                key={i}
                type="button"
                onClick={() => {
                  setMode('raw');
                  onChange(ex.expr);
                }}
                className="block w-full text-left px-2 py-1 rounded hover:bg-[var(--bg-secondary)] transition-colors group"
              >
                <span className="text-[10px] text-[var(--text-secondary)] group-hover:text-[var(--accent)]">{ex.label}:</span>
                <code className="text-[10px] font-mono text-[var(--text-tertiary)] ml-2">{ex.expr}</code>
              </button>
            ))}
          </div>
        </div>
      )}

      {mode === 'raw' ? (
        <div className="space-y-2">
          <input
            value={value}
            onChange={e => handleRawChange(e.target.value)}
            className="input font-mono text-[12px]"
            placeholder="e.g. metadata.replicas >= 2 && status == active"
          />
          {/* Quick insert buttons */}
          <div className="flex flex-wrap gap-1">
            {quickExamples.slice(0, 4).map((ex, i) => (
              <button
                key={i}
                type="button"
                onClick={() => onChange(ex.expr)}
                className="text-[9px] px-1.5 py-0.5 rounded bg-[var(--bg)] border border-[var(--border-light)] text-[var(--text-tertiary)] hover:text-[var(--accent)] hover:border-[var(--accent)]/30 transition-colors"
              >
                {ex.label}
              </button>
            ))}
          </div>
        </div>
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
                  else if (field === 'not_empty') updates.operator = 'not_empty';
                  // Reset operator if it's incompatible with the new field type
                  else if (isNumericOp(cond.operator) || isStringOp(cond.operator)) {
                    updates.operator = '==';
                  }
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
                  className="input text-[12px] w-[150px]"
                >
                  {basicOperators.map(o => (
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
              ) : cond.field === 'sync_status' ? (
                <select
                  value={cond.value}
                  onChange={e => updateCondition(i, { value: e.target.value })}
                  className="input text-[12px] flex-1"
                >
                  <option value="synced">synced</option>
                  <option value="pending">pending</option>
                  <option value="error">error</option>
                </select>
              ) : (
                <input
                  value={cond.value}
                  onChange={e => updateCondition(i, { value: e.target.value })}
                  className="input text-[12px] flex-1"
                  placeholder={isNumericOp(cond.operator) ? 'number' : isStringOp(cond.operator) ? 'substring' : 'value'}
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
    // field contains "value" / field starts_with "value"
    const strOpMatch = part.match(/^(\w[\w.]*)\s+(contains|starts_with)\s+(.+)$/);
    if (strOpMatch) {
      conditions.push({ field: strOpMatch[1], operator: strOpMatch[2], value: strOpMatch[3].replace(/"/g, '') });
      continue;
    }
    // field >= value / field <= value / field > value / field < value
    const numMatch = part.match(/^(\w[\w.]*)\s*(>=|<=|>|<)\s*(.+)$/);
    if (numMatch) {
      conditions.push({ field: numMatch[1], operator: numMatch[2], value: numMatch[3].replace(/"/g, '') });
      continue;
    }
    // field == value or field != value
    const match = part.match(/^(\w[\w.]*)\s*(==|!=)\s*(.+)$/);
    if (match) {
      conditions.push({ field: match[1], operator: match[2], value: match[3].replace(/"/g, '') });
      continue;
    }
  }

  return { conditions, logicOp };
}
