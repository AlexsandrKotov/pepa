'use client';

import { useState, useEffect, type ReactNode } from 'react';
import { scorecards, type Scorecard, type ScorecardRule, type ScorecardResult } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConfirmModal from '@/components/ConfirmModal';

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
      setToast({ message: 'Scorecard evaluated for all services', type: 'success' });
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
        <button onClick={() => setShowCreate(true)} className="btn btn-primary btn-sm">+ Create Scorecard</button>
      </div>

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
          <button onClick={() => setShowCreate(true)} className="btn btn-primary inline-block">+ Create Scorecard</button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {items.map((item) => (
            <div key={item.id} onClick={() => handleSelect(item)} className={`card p-4 modern-card-hover cursor-pointer transition-all ${selected?.id === item.id ? 'ring-1 ring-[var(--accent)]' : ''}`} style={{ borderRadius: '12px' }}>
              <div className="flex items-start justify-between">
                <p className="text-[14px] font-medium text-[var(--text-primary)]">{item.name}</p>
                <button
                  onClick={(e) => { e.stopPropagation(); setDeleteTarget({ type: 'scorecard', id: item.id, name: item.name }); }}
                  className="text-[var(--text-tertiary)] hover:text-red-500 transition-colors text-[12px]"
                  title="Delete scorecard"
                >✕</button>
              </div>
              <p className="text-[12px] text-[var(--text-tertiary)] mt-1">{item.description || 'No description'}</p>
              <div className="flex items-center gap-2 mt-3">
                <span className={`text-[10px] px-1.5 py-0.5 rounded ${item.enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>{item.enabled ? 'enabled' : 'disabled'}</span>
                <span className="text-[11px] text-[var(--text-tertiary)]">{item.rule_count || 0} rules</span>
              </div>
            </div>
          ))}
        </div>
      )}

      {selected && (
        <div className="space-y-6 page-animate-up page-delay-2">
          <div className="card" style={{ borderRadius: '12px' }}>
            <div className="card-header flex items-center justify-between">
              <span className="text-[13px] font-medium text-[var(--text-primary)]">Rules ({rules.length})</span>
              <div className="flex items-center gap-2">
                <button onClick={() => setShowRuleForm(v => !v)} className="btn btn-secondary btn-sm">+ Add Rule</button>
                <button onClick={handleEvaluate} disabled={evaluating} className="btn btn-primary btn-sm">{evaluating ? 'Evaluating...' : 'Evaluate All Services'}</button>
              </div>
            </div>

            {showRuleForm && (
              <div className="card-body space-y-3 border-b border-[var(--border-light)]">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                  <div><label className="label">Name *</label><input value={ruleForm.name} onChange={e => setRuleForm({ ...ruleForm, name: e.target.value })} className="input" placeholder="e.g. Has Health Endpoint" /></div>
                  <div><label className="label">Expression *</label><input value={ruleForm.expression} onChange={e => setRuleForm({ ...ruleForm, expression: e.target.value })} className="input font-mono" placeholder="e.g. metadata.health_endpoint == true" /></div>
                  <div><label className="label">Description</label><input value={ruleForm.description} onChange={e => setRuleForm({ ...ruleForm, description: e.target.value })} className="input" placeholder="What this rule checks" /></div>
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
                  <div><label className="label">Pass Message</label><input value={ruleForm.pass_message} onChange={e => setRuleForm({ ...ruleForm, pass_message: e.target.value })} className="input" placeholder="Shown when the rule passes" /></div>
                  <div><label className="label">Fail Message</label><input value={ruleForm.fail_message} onChange={e => setRuleForm({ ...ruleForm, fail_message: e.target.value })} className="input" placeholder="Shown when the rule fails" /></div>
                </div>
                <p className="text-[11px] text-[var(--text-tertiary)]">
                  Supported expressions: <code className="font-mono">metadata.field == value</code>, <code className="font-mono">field != null</code>, <code className="font-mono">has_metadata.field</code>, <code className="font-mono">not_empty.description</code>, <code className="font-mono">status == active</code>, <code className="font-mono">type_key == service</code>, combinable with <code className="font-mono">&&</code> / <code className="font-mono">||</code>
                </p>
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
              {rules.length === 0 && <div className="px-4 py-6 text-center text-[12px] text-[var(--text-tertiary)]">No rules defined — add rules to make this scorecard evaluate services</div>}
            </div>
          </div>

          {results.length > 0 && (
            <div className="card" style={{ borderRadius: '12px' }}>
              <div className="card-header"><span className="text-[13px] font-medium text-[var(--text-primary)]">Evaluation Results ({results.length})</span></div>
              <div className="table-container">
                <table className="w-full">
                  <thead><tr className="border-b border-[var(--border)]">
                    <th className="text-left text-[11px] text-[var(--text-tertiary)] font-medium px-4 py-2">Service</th>
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
