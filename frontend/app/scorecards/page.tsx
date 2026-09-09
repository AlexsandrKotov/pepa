'use client';

import { useState, useEffect, type ReactNode } from 'react';
import { scorecards, type Scorecard, type ScorecardRule, type ScorecardResult } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConfirmModal from '@/components/ConfirmModal';
import ExpressionBuilder from '@/components/ExpressionBuilder';

const levelBadge = (level: string) => {
  switch (level) {
    case 'platinum': return 'bg-indigo-500/10 text-indigo-500';
    case 'gold': return 'bg-amber-500/10 text-amber-600';
    case 'silver': return 'bg-slate-500/10 text-slate-500';
    case 'bronze': return 'bg-orange-500/10 text-orange-600';
    default: return 'bg-red-500/10 text-red-500';
  }
};

const severityBadge = (severity: string) => {
  switch (severity) {
    case 'critical': return 'bg-red-500/10 text-red-500';
    case 'warning': return 'bg-amber-500/10 text-amber-600';
    default: return 'bg-blue-500/10 text-blue-500';
  }
};

const emptyRuleForm = { name: '', description: '', expression: '', weight: 5, severity: 'warning', pass_message: '', fail_message: '' };

// ── Preset scorecard templates ────────────────────────────────

interface PresetRule {
  name: string;
  description: string;
  expression: string;
  weight: number;
  severity: string;
  pass_message: string;
  fail_message: string;
}

interface Preset {
  name: string;
  description: string;
  rules: PresetRule[];
}

const presets: Preset[] = [
  {
    name: 'Production Readiness',
    description: 'Checks if a service meets production readiness criteria',
    rules: [
      { name: 'Has Health Endpoint', description: 'Service exposes a health check endpoint', expression: 'has_metadata.health_endpoint', weight: 8, severity: 'critical', pass_message: 'Health endpoint configured', fail_message: 'No health endpoint found in metadata' },
      { name: 'Has Description', description: 'Service has a meaningful description', expression: 'not_empty.description', weight: 3, severity: 'warning', pass_message: 'Description provided', fail_message: 'Service description is empty or missing' },
      { name: 'Active Status', description: 'Service is marked as active', expression: 'status == active', weight: 5, severity: 'warning', pass_message: 'Service is active', fail_message: 'Service is not active' },
      { name: 'Has Owner', description: 'Service has an assigned owner', expression: 'has_metadata.owner', weight: 7, severity: 'critical', pass_message: 'Owner assigned', fail_message: 'No owner specified' },
      { name: 'Has Repository', description: 'Source code repository is linked', expression: 'has_metadata.repository', weight: 6, severity: 'warning', pass_message: 'Repository linked', fail_message: 'No repository linked' },
    ],
  },
  {
    name: 'Security Baseline',
    description: 'Verifies minimum security requirements',
    rules: [
      { name: 'Uses Vault Secrets', description: 'Service uses Vault for secret management', expression: 'has_metadata.vault_secrets', weight: 9, severity: 'critical', pass_message: 'Vault secrets configured', fail_message: 'No Vault secrets integration' },
      { name: 'Has Resource Limits', description: 'Container resource limits are set', expression: 'has_metadata.resource_limits', weight: 7, severity: 'warning', pass_message: 'Resource limits defined', fail_message: 'No resource limits set' },
      { name: 'Active Status', description: 'Service is active', expression: 'status == active', weight: 4, severity: 'info', pass_message: 'Service is active', fail_message: 'Service is not active' },
      { name: 'Has Monitoring', description: 'Monitoring is configured', expression: 'has_metadata.monitoring', weight: 8, severity: 'critical', pass_message: 'Monitoring configured', fail_message: 'No monitoring setup' },
    ],
  },
  {
    name: 'Best Practices',
    description: 'Checks adherence to platform best practices',
    rules: [
      { name: 'Has Documentation', description: 'Service has documentation links', expression: 'has_metadata.documentation', weight: 5, severity: 'info', pass_message: 'Documentation available', fail_message: 'No documentation linked' },
      { name: 'Has CI/CD', description: 'CI/CD pipeline is configured', expression: 'has_metadata.ci_cd', weight: 7, severity: 'warning', pass_message: 'CI/CD configured', fail_message: 'No CI/CD pipeline' },
      { name: 'Has Description', description: 'Service has a description', expression: 'not_empty.description', weight: 3, severity: 'info', pass_message: 'Description provided', fail_message: 'Description missing' },
      { name: 'Service Type', description: 'Entity is of type service', expression: 'type_key == service', weight: 2, severity: 'info', pass_message: 'Correct type', fail_message: 'Not a service type' },
    ],
  },
];

export default function ScorecardsPage() {
  const [items, setItems] = useState<Scorecard[]>([]);
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<Scorecard | null>(null);
  const [rules, setRules] = useState<ScorecardRule[]>([]);
  const [results, setResults] = useState<ScorecardResult[]>([]);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [evaluating, setEvaluating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [showRuleForm, setShowRuleForm] = useState(false);
  const [newName, setNewName] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [ruleForm, setRuleForm] = useState(emptyRuleForm);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{ type: 'scorecard' | 'rule'; id: string; name: string } | null>(null);
  const [showPresets, setShowPresets] = useState(false);
  const [togglingId, setTogglingId] = useState<string | null>(null);

  useEffect(() => { loadData(); }, []);

  const loadData = async () => {
    try {
      const data = await scorecards.list();
      setItems(data.scorecards || []);
    } catch { setToast({ message: 'Failed to load scorecards', type: 'error' }); }
    finally { setLoading(false); }
  };

  const handleSelect = async (item: Scorecard) => {
    setSelected(item);
    setExpanded(null);
    try {
      const [rulesData, resultsData] = await Promise.all([
        scorecards.get(item.id),
        scorecards.results(item.id),
      ]);
      setRules(rulesData.rules || []);
      setResults(resultsData.results || []);
    } catch { setToast({ message: 'Failed to load scorecard details', type: 'error' }); }
  };

  const refreshSelected = async (item: Scorecard) => {
    try {
      const [rulesData, resultsData] = await Promise.all([
        scorecards.get(item.id),
        scorecards.results(item.id),
      ]);
      setRules(rulesData.rules || []);
      setResults(resultsData.results || []);
    } catch { /* ignore */ }
  };

  const handleEvaluate = async () => {
    if (!selected) return;
    if (rules.length === 0) {
      setToast({ message: 'Add at least one rule before evaluating', type: 'error' });
      return;
    }
    setEvaluating(true);
    try {
      await scorecards.evaluateAll(selected.id);
      await refreshSelected(selected);
      setToast({ message: 'Scorecard evaluated for all entities', type: 'success' });
    } catch { setToast({ message: 'Evaluation failed', type: 'error' }); }
    finally { setEvaluating(false); }
  };

  const handleCreate = async () => {
    if (!newName.trim()) return;
    try {
      await scorecards.create({ name: newName, description: newDesc, enabled: true });
      setShowCreate(false);
      setNewName(''); setNewDesc('');
      loadData();
      setToast({ message: 'Scorecard created', type: 'success' });
    } catch { setToast({ message: 'Failed to create', type: 'error' }); }
  };

  const handleCreateFromPreset = async (preset: Preset) => {
    try {
      const sc = await scorecards.create({ name: preset.name, description: preset.description, enabled: true });
      // Add all rules from preset
      for (const rule of preset.rules) {
        await scorecards.addRule(sc.id, rule as unknown as Record<string, unknown>);
      }
      setShowPresets(false);
      loadData();
      setToast({ message: `Created "${preset.name}" from template`, type: 'success' });
      // Auto-select the new scorecard
      setTimeout(() => handleSelect(sc), 300);
    } catch { setToast({ message: 'Failed to create from template', type: 'error' }); }
  };

  const handleDuplicate = async (item: Scorecard) => {
    try {
      const dup = await scorecards.create({ name: `${item.name} (copy)`, description: item.description, enabled: false });
      // Copy rules
      const rulesData = await scorecards.get(item.id);
      for (const rule of (rulesData.rules || [])) {
        await scorecards.addRule(dup.id, {
          name: rule.name,
          description: rule.description,
          expression: rule.expression,
          weight: rule.weight,
          severity: rule.severity,
          pass_message: rule.pass_message,
          fail_message: rule.fail_message,
        });
      }
      loadData();
      setToast({ message: `Duplicated "${item.name}"`, type: 'success' });
    } catch { setToast({ message: 'Failed to duplicate', type: 'error' }); }
  };

  const handleToggleEnabled = async (item: Scorecard) => {
    setTogglingId(item.id);
    try {
      await scorecards.update(item.id, { enabled: !item.enabled });
      await loadData();
      if (selected?.id === item.id) {
        setSelected({ ...item, enabled: !item.enabled });
      }
    } catch { setToast({ message: 'Failed to toggle', type: 'error' }); }
    finally { setTogglingId(null); }
  };

  const handleAddRule = async () => {
    if (!selected || !ruleForm.name.trim() || !ruleForm.expression.trim()) return;
    try {
      await scorecards.addRule(selected.id, ruleForm);
      setRuleForm(emptyRuleForm);
      setShowRuleForm(false);
      await Promise.all([refreshSelected(selected), loadData()]);
      setToast({ message: 'Rule added', type: 'success' });
    } catch { setToast({ message: 'Failed to add rule (check expression and weight 1-10)', type: 'error' }); }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      if (deleteTarget.type === 'scorecard') {
        await scorecards.delete(deleteTarget.id);
        setSelected(null); setRules([]); setResults([]);
        await loadData();
      } else {
        await scorecards.deleteRule(deleteTarget.id);
        if (selected) await Promise.all([refreshSelected(selected), loadData()]);
      }
      setToast({ message: 'Deleted', type: 'success' });
    } catch { setToast({ message: 'Delete failed', type: 'error' }); }
    finally { setDeleteTarget(null); }
  };

  const scorePct = (r: ScorecardResult) => (r.max_score > 0 ? Math.round((r.score / r.max_score) * 100) : 0);

  // Summary stats
  const avgScore = results.length > 0
    ? Math.round(results.reduce((sum, r) => sum + scorePct(r), 0) / results.length)
    : 0;
  const levelCounts = results.reduce((acc, r) => {
    acc[r.level] = (acc[r.level] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);

  if (loading) return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <h1 className="page-title-modern">Scorecards</h1>
        <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}><p className="text-[13px] text-[var(--text-tertiary)]">Loading...</p></div>
      </div>
    </div>
  );

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
        <ConfirmModal
          open={deleteTarget !== null}
          title={deleteTarget?.type === 'scorecard' ? 'Delete Scorecard' : 'Delete Rule'}
          description={deleteTarget ? `Are you sure you want to delete "${deleteTarget.name}"? This cannot be undone.` : ''}
          confirmLabel="Delete"
          variant="danger"
          onConfirm={handleDelete}
          onCancel={() => setDeleteTarget(null)}
        />
        <div className="page-animate flex items-center justify-between">
          <div><h1 className="page-title-modern">Scorecards</h1><p className="page-subtitle-modern">Service quality scorecards with automated evaluation</p></div>
          <div className="flex items-center gap-2">
            <button onClick={() => setShowPresets(true)} className="btn btn-secondary btn-sm">From Template</button>
            <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm">+ Create Scorecard</button>
          </div>
        </div>

        {/* Presets modal */}
        {showPresets && (
          <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
            <div className="card-header flex items-center justify-between">
              <span className="text-[13px] font-medium text-[var(--text-primary)]">Scorecard Templates</span>
              <button onClick={() => setShowPresets(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-[12px]">✕</button>
            </div>
            <div className="card-body">
              <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                {presets.map((preset) => (
                  <div key={preset.name} className="p-3 rounded-lg border border-[var(--border-light)] hover:border-[var(--border)] transition-all">
                    <p className="text-[13px] font-medium text-[var(--text-primary)]">{preset.name}</p>
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-1">{preset.description}</p>
                    <p className="text-[10px] text-[var(--text-tertiary)] mt-2">{preset.rules.length} rules</p>
                    <button onClick={() => handleCreateFromPreset(preset)} className="btn btn-primary btn-sm mt-2 w-full text-[11px]">Use Template</button>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}

        {showCreate && (
          <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
            <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">New Scorecard</span></div>
            <div className="card-body space-y-3">
              <div><label className="label">Name</label><input value={newName} onChange={e => setNewName(e.target.value)} className="input" placeholder="e.g. Production Readiness" /></div>
              <div><label className="label">Description</label><input value={newDesc} onChange={e => setNewDesc(e.target.value)} className="input" placeholder="Description" /></div>
              <div className="flex gap-2"><button onClick={handleCreate} className="btn btn-primary btn-sm">Create</button><button onClick={() => setShowCreate(false)} className="btn btn-secondary btn-sm">Cancel</button></div>
            </div>
          </div>
        )}

        {items.length === 0 ? (
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <div className="text-4xl mb-3 opacity-30">📋</div>
            <p className="text-[14px] font-medium text-[var(--text-primary)]">No scorecards yet</p>
            <p className="text-[12px] text-[var(--text-tertiary)] mt-1 mb-4">Create a scorecard to evaluate service quality against best practices</p>
            <div className="flex gap-2 justify-center">
              <button onClick={() => setShowCreate(true)} className="btn btn-primary">+ Create Scorecard</button>
              <button onClick={() => setShowPresets(true)} className="btn btn-secondary">From Template</button>
            </div>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {items.map((item) => (
              <div key={item.id} onClick={() => handleSelect(item)} className={`card p-4 modern-card-hover cursor-pointer transition-all ${selected?.id === item.id ? 'ring-1 ring-[var(--accent)]' : ''}`} style={{ borderRadius: '12px' }}>
                <div className="flex items-start justify-between">
                  <p className="text-[14px] font-medium text-[var(--text-primary)]">{item.name}</p>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={(e) => { e.stopPropagation(); handleToggleEnabled(item); }}
                      disabled={togglingId === item.id}
                      className={`text-[10px] px-1.5 py-0.5 rounded transition-colors ${item.enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}
                      title={item.enabled ? 'Click to disable' : 'Click to enable'}
                    >
                      {item.enabled ? 'enabled' : 'disabled'}
                    </button>
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDuplicate(item); }}
                      className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors text-[11px]"
                      title="Duplicate"
                    >⧉</button>
                    <button
                      onClick={(e) => { e.stopPropagation(); setDeleteTarget({ type: 'scorecard', id: item.id, name: item.name }); }}
                      className="text-[var(--text-tertiary)] hover:text-red-500 transition-colors text-[12px]"
                      title="Delete scorecard"
                    >✕</button>
                  </div>
                </div>
                <p className="text-[12px] text-[var(--text-tertiary)] mt-1">{item.description || 'No description'}</p>
                <div className="flex items-center gap-2 mt-3">
                  <span className="text-[11px] text-[var(--text-tertiary)]">{item.rule_count || 0} rules</span>
                </div>
              </div>
            ))}
          </div>
        )}

        {selected && (
          <div className="space-y-6 page-animate-up page-delay-2">
            {/* Summary stats */}
            {results.length > 0 && (
              <div className="card" style={{ borderRadius: '12px' }}>
                <div className="card-body">
                  <div className="flex items-center gap-6">
                    <div>
                      <span className="text-[20px] font-semibold text-[var(--text-primary)]">{avgScore}%</span>
                      <span className="text-[12px] text-[var(--text-tertiary)] ml-2">avg score</span>
                    </div>
                    <div className="flex gap-3">
                      {['platinum', 'gold', 'silver', 'bronze'].map(level => (
                        <div key={level} className="flex items-center gap-1">
                          <span className={`text-[10px] px-1.5 py-0.5 rounded ${levelBadge(level)}`}>{level}</span>
                          <span className="text-[12px] text-[var(--text-secondary)]">{levelCounts[level] || 0}</span>
                        </div>
                      ))}
                    </div>
                    <span className="text-[12px] text-[var(--text-tertiary)] ml-auto">{results.length} entities evaluated</span>
                  </div>
                </div>
              </div>
            )}

            <div className="card" style={{ borderRadius: '12px' }}>
              <div className="card-header flex items-center justify-between">
                <span className="text-[13px] font-medium text-[var(--text-primary)]">Rules ({rules.length})</span>
                <div className="flex items-center gap-2">
                  <button onClick={() => setShowRuleForm(v => !v)} className="btn btn-secondary btn-sm">+ Add Rule</button>
                  <button onClick={handleEvaluate} disabled={evaluating} className="btn btn-primary btn-sm">{evaluating ? 'Evaluating...' : 'Evaluate All'}</button>
                </div>
              </div>

              {showRuleForm && (
                <div className="card-body space-y-3 border-b border-[var(--border-light)]">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <div><label className="label">Name *</label><input value={ruleForm.name} onChange={e => setRuleForm({ ...ruleForm, name: e.target.value })} className="input" placeholder="e.g. Has Health Endpoint" /></div>
                    <div className="grid grid-cols-2 gap-3">
                      <div><label className="label">Weight (1-10)</label><input type="number" min={1} max={10} value={ruleForm.weight} onChange={e => setRuleForm({ ...ruleForm, weight: Number(e.target.value) })} className="input" /></div>
                      <div><label className="label">Severity</label>
                        <select value={ruleForm.severity} onChange={e => setRuleForm({ ...ruleForm, severity: e.target.value })} className="input">
                          <option value="info">info</option>
                          <option value="warning">warning</option>
                          <option value="critical">critical</option>
                        </select>
                      </div>
                    </div>
                    <div className="col-span-1 md:col-span-2">
                      <label className="label">Expression *</label>
                      <ExpressionBuilder value={ruleForm.expression} onChange={(expr) => setRuleForm({ ...ruleForm, expression: expr })} />
                    </div>
                    <div><label className="label">Description</label><input value={ruleForm.description} onChange={e => setRuleForm({ ...ruleForm, description: e.target.value })} className="input" placeholder="What this rule checks" /></div>
                    <div className="grid grid-cols-2 gap-3">
                      <div><label className="label">Pass Message</label><input value={ruleForm.pass_message} onChange={e => setRuleForm({ ...ruleForm, pass_message: e.target.value })} className="input" placeholder="Shown when rule passes" /></div>
                      <div><label className="label">Fail Message</label><input value={ruleForm.fail_message} onChange={e => setRuleForm({ ...ruleForm, fail_message: e.target.value })} className="input" placeholder="Shown when rule fails" /></div>
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button onClick={handleAddRule} className="btn btn-primary btn-sm">Add Rule</button>
                    <button onClick={() => { setShowRuleForm(false); setRuleForm(emptyRuleForm); }} className="btn btn-secondary btn-sm">Cancel</button>
                  </div>
                </div>
              )}

              <div className="divide-y divide-[var(--border-light)]">
                {rules.map((r) => (
                  <div key={r.id} className="px-4 py-2.5 flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="text-[12px] font-medium text-[var(--text-primary)]">{r.name}</p>
                        <span className={`text-[9px] px-1.5 py-0.5 rounded ${severityBadge(r.severity)}`}>{r.severity}</span>
                      </div>
                      <p className="text-[11px] text-[var(--text-tertiary)] truncate">{r.description || r.expression}</p>
                      <p className="text-[10px] text-[var(--text-tertiary)] font-mono truncate">{r.expression}</p>
                    </div>
                    <div className="flex items-center gap-3 shrink-0">
                      <span className="text-[11px] text-[var(--text-secondary)]">{r.weight}x</span>
                      <button
                        onClick={() => setDeleteTarget({ type: 'rule', id: r.id, name: r.name })}
                        className="text-[var(--text-tertiary)] hover:text-red-500 transition-colors text-[12px]"
                        title="Delete rule"
                      >✕</button>
                    </div>
                  </div>
                ))}
                {rules.length === 0 && <div className="px-4 py-6 text-center text-[12px] text-[var(--text-tertiary)]">No rules defined — add rules to make this scorecard evaluate entities</div>}
              </div>
            </div>

            {results.length > 0 && (
              <div className="card" style={{ borderRadius: '12px' }}>
                <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">Evaluation Results ({results.length})</span></div>
                <div className="table-container">
                  <table className="w-full">
                    <thead><tr className="border-b border-[var(--border)]">
                      <th className="text-left text-[11px] text-[var(--text-tertiary)] font-medium px-4 py-2">Entity</th>
                      <th className="text-center text-[11px] text-[var(--text-tertiary)] font-medium px-4 py-2">Level</th>
                      <th className="text-center text-[11px] text-[var(--text-tertiary)] font-medium px-4 py-2">Score</th>
                      <th className="text-center text-[11px] text-[var(--text-tertiary)] font-medium px-4 py-2">Rules Passed</th>
                      <th className="text-left text-[11px] text-[var(--text-tertiary)] font-medium px-4 py-2">Evaluated</th>
                    </tr></thead>
                    <tbody>
                      {results.map((r) => {
                        const pct = scorePct(r);
                        const isOpen = expanded === r.id;
                        const details = Array.isArray(r.details) ? r.details : [];
                        return (
                          <FragmentRow key={r.id}>
                            <tr onClick={() => setExpanded(isOpen ? null : r.id)} className="border-b border-[var(--border-light)] cursor-pointer hover:bg-[var(--bg)]">
                              <td className="px-4 py-2 text-[12px] font-medium text-[var(--text-primary)]">{r.entity_name || String(r.entity_id).slice(0, 8)}</td>
                              <td className="px-4 py-2 text-center"><span className={`text-[10px] px-1.5 py-0.5 rounded ${levelBadge(r.level)}`}>{r.level}</span></td>
                              <td className="px-4 py-2 text-center"><span className={`text-[13px] font-semibold ${pct >= 75 ? 'text-green-600' : pct >= 50 ? 'text-yellow-600' : 'text-red-600'}`}>{pct}%</span></td>
                              <td className="px-4 py-2 text-center text-[12px] text-[var(--text-secondary)]">{r.pass_count}/{r.total_rules}</td>
                              <td className="px-4 py-2 text-[11px] text-[var(--text-tertiary)]">{r.evaluated_at ? new Date(String(r.evaluated_at)).toLocaleString() : '-'}</td>
                            </tr>
                            {isOpen && details.length > 0 && (
                              <tr className="border-b border-[var(--border-light)]">
                                <td colSpan={5} className="px-4 py-3 bg-[var(--bg)]">
                                  <div className="space-y-1.5">
                                    {details.map((d, di) => (
                                      <div key={di} className="flex items-start gap-2">
                                        <span className={`text-[11px] shrink-0 ${d.passed ? 'text-emerald-600' : 'text-red-500'}`}>{d.passed ? '✓' : '✕'}</span>
                                        <span className="text-[11px] text-[var(--text-primary)] font-medium shrink-0">{d.rule_name}</span>
                                        <span className="text-[11px] text-[var(--text-tertiary)]">{d.message}</span>
                                        <span className="text-[10px] text-[var(--text-tertiary)] ml-auto shrink-0">{d.score}/{d.weight}</span>
                                      </div>
                                    ))}
                                  </div>
                                </td>
                              </tr>
                            )}
                          </FragmentRow>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function FragmentRow({ children }: { children: ReactNode }) {
  return <>{children}</>;
}
