'use client';
import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { jira, plugins, type JiraIssue, type JiraComment, type JiraAutomationRule, type JiraFilters, type JiraStats } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConfirmModal from '@/components/ConfirmModal';

const priorityColors: Record<string, string> = {
  Critical: 'bg-red-500/10 text-red-500 border-red-500/20',
  Highest: 'bg-red-500/10 text-red-500 border-red-500/20',
  High: 'bg-orange-500/10 text-orange-600 border-orange-500/20',
  Medium: 'bg-amber-500/10 text-amber-600 border-amber-500/20',
  Low: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  Lowest: 'bg-[var(--bg)] text-[var(--text-secondary)] border-[var(--border)]',
};

const statusColors: Record<string, string> = {
  'To Do': 'bg-[var(--border-light)] text-[var(--text-secondary)]',
  'Open': 'bg-[var(--border-light)] text-[var(--text-secondary)]',
  'In Progress': 'bg-blue-500/10 text-blue-500',
  'In Review': 'bg-violet-500/10 text-violet-500',
  'Done': 'bg-emerald-500/15 text-emerald-600',
  'Closed': 'bg-emerald-500/15 text-emerald-600',
  'Rejected': 'bg-red-500/15 text-red-500',
};

const typeIcons: Record<string, string> = {
  Story: '\u{1F4D6}',
  Task: '\u2705',
  Bug: '\u{1F41B}',
  Epic: '\u{1F3D4}',
  'Sub-task': '\u{1F517}',
  Spike: '\u26A1',
};

const triggerTypes = [
  { value: 'deployment_created', label: 'Deployment Created' },
  { value: 'deployment_succeeded', label: 'Deployment Succeeded' },
  { value: 'deployment_failed', label: 'Deployment Failed' },
  { value: 'pipeline_completed', label: 'Pipeline Completed' },
  { value: 'service_created', label: 'Service Created' },
  { value: 'manual', label: 'Manual' },
];

const actionTypes = [
  { value: 'add_comment', label: 'Add Comment' },
  { value: 'transition', label: 'Transition Issue' },
  { value: 'update_field', label: 'Update Field' },
  { value: 'notify', label: 'Send Notification' },
];

export default function JiraPage() {
  const [pluginInstalled, setPluginInstalled] = useState(false);
  const [checkingPlugin, setCheckingPlugin] = useState(true);
  const [issues, setIssues] = useState<JiraIssue[]>([]);
  const [total, setTotal] = useState(0);
  const [stats, setStats] = useState<JiraStats | null>(null);
  const [labels, setLabels] = useState<string[]>([]);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'issues' | 'automation'>('issues');

  // Filters
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [filters, setFilters] = useState<JiraFilters>({});
  const [searchText, setSearchText] = useState('');

  // Issue detail
  const [selectedIssue, setSelectedIssue] = useState<JiraIssue | null>(null);
  const [comments, setComments] = useState<JiraComment[]>([]);
  const [commentText, setCommentText] = useState('');
  const [transitions, setTransitions] = useState<{ id: string; name: string; to: string }[]>([]);

  // Create issue
  const [createOpen, setCreateOpen] = useState(false);
  const [newIssue, setNewIssue] = useState({ summary: '', project_key: '', issue_type: 'Task', description: '', priority: 'Medium', assignee: '', labels: '' as string });

  // Automation
  const [rules, setRules] = useState<JiraAutomationRule[]>([]);
  const [ruleModalOpen, setRuleModalOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<Partial<JiraAutomationRule> | null>(null);
  const [deleteRuleConfirm, setDeleteRuleConfirm] = useState<string | null>(null);

  // Escape key closes modals/panels
  const anyModalOpen = selectedIssue !== null || createOpen || ruleModalOpen || deleteRuleConfirm !== null;
  useEscapeKey(() => {
    if (deleteRuleConfirm) setDeleteRuleConfirm(null);
    else if (ruleModalOpen) { setRuleModalOpen(false); setEditingRule(null); }
    else if (createOpen) setCreateOpen(false);
    else if (selectedIssue) setSelectedIssue(null);
  }, anyModalOpen);

  // Check if Jira plugin is installed
  useEffect(() => {
    plugins.list()
      .then(d => {
        // After install the backend marks loaded plugins as status 'running';
        // accept both statuses (same rule as the Plugins page).
        const jiraPlugin = (d.plugins || []).find(p => p.name === 'jira' && (p.status === 'installed' || p.status === 'running'));
        setPluginInstalled(!!jiraPlugin);
      })
      .catch(() => setPluginInstalled(false))
      .finally(() => setCheckingPlugin(false));
  }, []);

  const refresh = useCallback(async () => {
    try {
      const data = await jira.list(filters);
      setIssues(data.issues || []);
      setTotal(data.total || 0);
    } catch { /* ignore */ }
    setLoading(false);
  }, [filters]);

  useEffect(() => { refresh(); }, [refresh]);

  useEffect(() => {
    jira.getStats().then(setStats).catch(() => {});
    jira.getLabels().then(d => setLabels(d.labels || [])).catch(() => {});
  }, [issues]);

  const handleSync = async () => {
    setSyncing(true);
    try {
      const result = await jira.sync();
      setToast({ message: `Synced ${result.synced} issues from Jira`, type: 'success' });
      await refresh();
    } catch (e: unknown) {
      setToast({ message: String(e instanceof Error ? e.message : 'Sync failed'), type: 'error' });
    }
    setSyncing(false);
  };

  const handleSearch = () => {
    setFilters(f => ({ ...f, search: searchText || undefined }));
  };

  const openIssueDetail = async (issue: JiraIssue) => {
    setSelectedIssue(issue);
    try {
      const [commentsData, transData] = await Promise.all([
        jira.getComments(issue.id),
        jira.getTransitions(issue.id),
      ]);
      setComments(commentsData.comments || []);
      setTransitions(transData.transitions || []);
    } catch { /* ignore */ }
  };

  const handleAddComment = async () => {
    if (!selectedIssue || !commentText.trim()) return;
    try {
      await jira.addComment(selectedIssue.id, commentText);
      setCommentText('');
      const data = await jira.getComments(selectedIssue.id);
      setComments(data.comments || []);
      setToast({ message: 'Comment added', type: 'success' });
    } catch {
      setToast({ message: 'Failed to add comment', type: 'error' });
    }
  };

  const handleTransition = async (status: string) => {
    if (!selectedIssue) return;
    try {
      await jira.transition(selectedIssue.id, status);
      setToast({ message: `Issue transitioned to ${status}`, type: 'success' });
      await refresh();
      openIssueDetail({ ...selectedIssue, status });
    } catch {
      setToast({ message: 'Failed to transition', type: 'error' });
    }
  };

  const handleCreateIssue = async () => {
    try {
      await jira.create({
        summary: newIssue.summary,
        project_key: newIssue.project_key,
        issue_type: newIssue.issue_type,
        description: newIssue.description,
        priority: newIssue.priority,
        assignee: newIssue.assignee,
        labels: newIssue.labels ? newIssue.labels.split(',').map(l => l.trim()) : [],
      });
      setToast({ message: 'Issue created', type: 'success' });
      setCreateOpen(false);
      setNewIssue({ summary: '', project_key: '', issue_type: 'Task', description: '', priority: 'Medium', assignee: '', labels: '' });
      await refresh();
    } catch {
      setToast({ message: 'Failed to create issue', type: 'error' });
    }
  };

  // Automation rules
  const loadRules = async () => {
    try {
      const data = await jira.getAutomationRules();
      setRules(data.rules || []);
    } catch { /* ignore */ }
  };

  useEffect(() => { if (activeTab === 'automation') loadRules(); }, [activeTab]);

  const handleSaveRule = async () => {
    if (!editingRule) return;
    try {
      if (editingRule.id) {
        await jira.updateAutomationRule(editingRule.id, editingRule);
      } else {
        await jira.createAutomationRule(editingRule);
      }
      setToast({ message: 'Rule saved', type: 'success' });
      setRuleModalOpen(false);
      setEditingRule(null);
      await loadRules();
    } catch {
      setToast({ message: 'Failed to save rule', type: 'error' });
    }
  };

  const handleDeleteRule = async () => {
    if (!deleteRuleConfirm) return;
    try {
      await jira.deleteAutomationRule(deleteRuleConfirm);
      setToast({ message: 'Rule deleted', type: 'success' });
      setDeleteRuleConfirm(null);
      await loadRules();
    } catch {
      setToast({ message: 'Failed to delete rule', type: 'error' });
    }
  };

  const toggleFilter = (key: keyof JiraFilters, value: string) => {
    setFilters(f => {
      const arr = (f[key] as string[] | undefined) || [];
      const next = arr.includes(value) ? arr.filter(v => v !== value) : [...arr, value];
      return { ...f, [key]: next.length > 0 ? next : undefined };
    });
  };

  const clearFilters = () => {
    setFilters({});
    setSearchText('');
  };

  const activeFilterCount = [
    filters.issue_types?.length,
    filters.statuses?.length,
    filters.labels?.length,
    filters.priorities?.length,
    filters.assignee ? 1 : 0,
    filters.project_key ? 1 : 0,
  ].filter(Boolean).reduce((a, b) => (a || 0) + (b || 0), 0) || 0;

  const displayIssues = issues;

  if (checkingPlugin) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6">
          <h1 className="page-title-modern">Jira Integration</h1>
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <p className="text-[13px] text-[var(--text-secondary)]">Checking plugin status...</p>
          </div>
        </div>
      </div>
    );
  }

  if (!pluginInstalled) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6">
          <h1 className="page-title-modern">Jira Integration</h1>
          <div className="card card-body text-center py-16" style={{ borderRadius: '12px' }}>
            <div className="text-4xl mb-3 opacity-30">🔌</div>
            <h3 className="text-[15px] font-medium text-[var(--text-primary)] mb-2">Jira plugin is not installed</h3>
            <p className="text-[13px] text-[var(--text-secondary)] mb-4">
              Install the Jira plugin from the Marketplace to manage issues, automate workflows, and track deployments.
            </p>
            <a href="/marketplace" className="btn btn-primary">
              Go to Marketplace
            </a>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Stats */}
      <div className="grid grid-cols-5 gap-4 page-animate">
        {[
          { label: 'Total Issues', value: stats?.total ?? total, color: 'text-blue-400' },
          { label: 'In Progress', value: (stats?.by_status?.['In Progress'] || 0) + (stats?.by_status?.['In Review'] || 0), color: 'text-amber-400' },
          { label: 'Done', value: stats?.by_status?.['Done'] || 0, color: 'text-emerald-400' },
          { label: 'Open Bugs', value: stats?.open_bugs || 0, color: 'text-red-400' },
          { label: 'Closed', value: (stats?.by_status?.['Closed'] || 0) + (stats?.by_status?.['Done'] || 0), color: 'text-gray-400' },
        ].map(s => (
          <div key={s.label} className="modern-stat-card">
            <div className={`text-2xl font-bold ${s.color}`}>{s.value}</div>
            <div className="text-xs text-[var(--text-tertiary)] mt-1">{s.label}</div>
          </div>
        ))}
      </div>

      {/* Header + Tabs */}
      <div className="page-animate flex items-center justify-between">
        <div className="flex items-center gap-6">
          <div>
            <h1 className="page-title-modern">Jira Integration</h1>
            <p className="page-subtitle-modern">Manage issues, automate workflows, track deployments</p>
          </div>
          <div className="flex gap-1 bg-[var(--bg)] rounded-lg p-0.5">
            <button onClick={() => setActiveTab('issues')} className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${activeTab === 'issues' ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}>Issues</button>
            <button onClick={() => setActiveTab('automation')} className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${activeTab === 'automation' ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}>Automation</button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button onClick={() => setCreateOpen(true)} className="px-3 py-1.5 bg-[var(--surface)] border border-[var(--border)] text-[var(--text-primary)] text-sm rounded-lg hover:bg-[var(--bg)]">
            + Create Issue
          </button>
          <button onClick={handleSync} disabled={syncing} className="px-3 py-1.5 bg-[var(--accent)] text-white text-sm rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity">
            {syncing ? 'Syncing...' : 'Sync from Jira'}
          </button>
        </div>
      </div>

      {activeTab === 'issues' && (
        <>
          {/* Search + Filter Toggle */}
          <div className="flex items-center gap-3 page-animate-up page-delay-1">
            <div className="flex-1 flex items-center gap-2 bg-[var(--surface)] border border-[var(--border)] rounded-lg px-3 py-2">
              <svg className="w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
              <input
                type="text"
                placeholder="Search issues by key, summary, or description..."
                value={searchText}
                onChange={e => setSearchText(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleSearch()}
                className="flex-1 text-sm text-[var(--text-primary)] placeholder-[#a3a3a3] outline-none bg-transparent"
              />
              {searchText && (
                <button onClick={() => { setSearchText(''); setFilters(f => ({ ...f, search: undefined })); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
                </button>
              )}
            </div>
            <button
              onClick={() => setFiltersOpen(!filtersOpen)}
              className={`flex items-center gap-2 px-3 py-2 border rounded-lg text-sm transition-colors ${filtersOpen || activeFilterCount > 0 ? 'border-blue-500/20 bg-blue-500/10 text-[var(--accent)]' : 'border-[var(--border)] bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--bg)]'}`}
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" /></svg>
              Filters
              {activeFilterCount > 0 && <span className="bg-[var(--accent)] text-white text-[10px] px-1.5 py-0.5 rounded-full">{activeFilterCount}</span>}
            </button>
            {activeFilterCount > 0 && (
              <button onClick={clearFilters} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">Clear all</button>
            )}
          </div>

          {/* Collapsible Filter Panel */}
          {filtersOpen && (
            <div className="bg-[var(--surface)] rounded-xl border border-[var(--border)] p-4 space-y-4 page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
              <div className="grid grid-cols-3 gap-4">
                {/* Issue Type */}
                <div>
                  <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Issue Type</div>
                  <div className="flex flex-wrap gap-1.5">
                    {['Story', 'Task', 'Bug', 'Epic', 'Sub-task', 'Spike'].map(t => (
                      <button key={t} onClick={() => toggleFilter('issue_types', t)} className={`text-xs px-2 py-1 rounded-md border transition-colors ${filters.issue_types?.includes(t) ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--bg)]'}`}>
                        {typeIcons[t] || '\u{1F4C4}'} {t}
                      </button>
                    ))}
                  </div>
                </div>
                {/* Status */}
                <div>
                  <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Status</div>
                  <div className="flex flex-wrap gap-1.5">
                    {['Open', 'To Do', 'In Progress', 'In Review', 'Done', 'Closed', 'Rejected'].map(s => (
                      <button key={s} onClick={() => toggleFilter('statuses', s)} className={`text-xs px-2 py-1 rounded-md border transition-colors ${filters.statuses?.includes(s) ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--bg)]'}`}>
                        {s}
                      </button>
                    ))}
                  </div>
                </div>
                {/* Priority */}
                <div>
                  <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Priority</div>
                  <div className="flex flex-wrap gap-1.5">
                    {['Critical', 'High', 'Medium', 'Low', 'Lowest'].map(p => (
                      <button key={p} onClick={() => toggleFilter('priorities', p)} className={`text-xs px-2 py-1 rounded-md border transition-colors ${filters.priorities?.includes(p) ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--bg)]'}`}>
                        {p}
                      </button>
                    ))}
                  </div>
                </div>
              </div>
              {/* Labels */}
              {labels.length > 0 && (
                <div>
                  <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Labels</div>
                  <div className="flex flex-wrap gap-1.5">
                    {labels.map(l => (
                      <button key={l} onClick={() => toggleFilter('labels', l)} className={`text-[10px] px-2 py-0.5 rounded-md border transition-colors ${filters.labels?.includes(l) ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--bg)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--border)]'}`}>
                        {l}
                      </button>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Issues Table */}
          {loading ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center page-animate-up page-delay-2">
              <div className="text-[var(--text-tertiary)] text-sm">Loading issues...</div>
            </div>
          ) : displayIssues.length === 0 ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center page-animate-up page-delay-2">
              <div className="text-[var(--text-tertiary)] text-sm">
                {total === 0
                  ? 'No Jira issues synced yet. Click "Sync from Jira" to pull issues.'
                  : 'No issues match the selected filters.'}
              </div>
              {activeFilterCount > 0 && (
                <button onClick={clearFilters} className="mt-2 text-xs text-[var(--accent)] hover:underline">Clear filters</button>
              )}
            </div>
          ) : (
            <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] overflow-hidden page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
              <table className="w-full text-sm">
                <thead className="bg-[var(--bg)] border-b border-[var(--border)]">
                  <tr>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Key</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Summary</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Type</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Priority</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Status</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Assignee</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Labels</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border-light)]">
                  {displayIssues.map(issue => (
                    <tr key={issue.id} onClick={() => openIssueDetail(issue)} className="hover:bg-[var(--bg)] cursor-pointer">
                      <td className="px-4 py-3">
                        <span className="font-mono text-xs font-medium text-[var(--accent)]">{issue.issue_key}</span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-[13px] text-[var(--text-primary)] truncate max-w-[300px]">{issue.summary}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-xs">{typeIcons[issue.issue_type] || '\u{1F4C4}'} {issue.issue_type}</span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`text-xs px-2 py-0.5 rounded-full border ${priorityColors[issue.priority] || 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]'}`}>
                          {issue.priority}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`text-xs px-2 py-0.5 rounded-full ${statusColors[issue.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>
                          {issue.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-xs text-[var(--text-secondary)]">{issue.assignee || '-'}</td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1 flex-wrap">
                          {(issue.labels || []).slice(0, 3).map(l => (
                            <span key={l} className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)]">{l}</span>
                          ))}
                          {(issue.labels || []).length > 3 && (
                            <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-tertiary)]">+{issue.labels.length - 3}</span>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      {/* Automation Tab */}
      {activeTab === 'automation' && (
        <div className="space-y-4 page-animate-up page-delay-1">
          <div className="flex items-center justify-between">
            <p className="text-sm text-[var(--text-secondary)]">Automate Jira actions when PEPA events occur. Auto-comment on issues when deployments happen, auto-transition statuses, and more.</p>
            <button onClick={() => { setEditingRule({ name: '', description: '', trigger_type: 'deployment_succeeded', jira_project_key: '', jql_filter: '', action_type: 'add_comment', action_config: {}, enabled: true }); setRuleModalOpen(true); }} className="px-3 py-1.5 bg-[var(--accent)] text-white text-sm rounded-lg hover:opacity-90 transition-opacity">
              + New Rule
            </button>
          </div>

          {rules.length === 0 ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center">
              <div className="text-2xl mb-2">\u2699\uFE0F</div>
              <div className="text-[var(--text-tertiary)] text-sm">No automation rules yet. Create one to start automating Jira workflows.</div>
            </div>
          ) : (
            <div className="space-y-3">
              {rules.map(rule => (
                <div key={rule.id} className="bg-[var(--surface)] rounded-xl border border-[var(--border)] p-4 flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className={`w-2 h-2 rounded-full ${rule.enabled ? 'bg-emerald-500' : 'bg-gray-300'}`} />
                    <div>
                      <div className="text-sm font-medium text-[var(--text-primary)]">{rule.name}</div>
                      <div className="text-xs text-[var(--text-tertiary)] mt-0.5">
                        When <span className="font-medium text-[var(--text-secondary)]">{triggerTypes.find(t => t.value === rule.trigger_type)?.label || rule.trigger_type}</span>
                        {' \u2192 '}
                        <span className="font-medium text-[var(--text-secondary)]">{actionTypes.find(a => a.value === rule.action_type)?.label || rule.action_type}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    {rule.last_triggered_at && (
                      <span className="text-[10px] text-[var(--text-tertiary)]">Last: {new Date(rule.last_triggered_at).toLocaleDateString()}</span>
                    )}
                    <button onClick={() => { setEditingRule(rule); setRuleModalOpen(true); }} className="text-xs text-[var(--accent)] hover:underline">Edit</button>
                    <button onClick={() => setDeleteRuleConfirm(rule.id)} className="text-xs text-red-500 hover:underline">Delete</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Issue Detail Side Panel */}
      {selectedIssue && (
        <div className="fixed inset-0 z-50 flex">
          <div className="absolute inset-0 bg-black/20 backdrop-blur-sm" onClick={() => setSelectedIssue(null)} />
          <div className="relative ml-auto w-full max-w-lg bg-[var(--surface)] h-full overflow-y-auto shadow-2xl border-l border-[var(--border)]">
            <div className="sticky top-0 bg-[var(--surface)] border-b border-[var(--border)] px-6 py-4 flex items-center justify-between z-10">
              <div className="flex items-center gap-3">
                <span className="font-mono text-sm font-medium text-[var(--accent)]">{selectedIssue.issue_key}</span>
                <span className={`text-xs px-2 py-0.5 rounded-full ${statusColors[selectedIssue.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>{selectedIssue.status}</span>
              </div>
              <button onClick={() => setSelectedIssue(null)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
              </button>
            </div>

            <div className="p-6 space-y-6">
              {/* Summary & Type */}
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-sm">{typeIcons[selectedIssue.issue_type] || '\u{1F4C4}'} {selectedIssue.issue_type}</span>
                  <span className={`text-xs px-2 py-0.5 rounded-full border ${priorityColors[selectedIssue.priority] || 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]'}`}>{selectedIssue.priority}</span>
                </div>
                <h2 className="text-base font-semibold text-[var(--text-primary)]">{selectedIssue.summary}</h2>
              </div>

              {/* Meta */}
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div><span className="text-[var(--text-tertiary)]">Assignee:</span> <span className="text-[var(--text-primary)] ml-1">{selectedIssue.assignee || 'Unassigned'}</span></div>
                <div><span className="text-[var(--text-tertiary)]">Reporter:</span> <span className="text-[var(--text-primary)] ml-1">{selectedIssue.reporter || '-'}</span></div>
                <div><span className="text-[var(--text-tertiary)]">Project:</span> <span className="text-[var(--text-primary)] ml-1">{selectedIssue.project_key}</span></div>
                {selectedIssue.parent_key && <div><span className="text-[var(--text-tertiary)]">Parent:</span> <span className="text-[var(--accent)] ml-1">{selectedIssue.parent_key}</span></div>}
                {selectedIssue.linked_mr_url && <div className="col-span-2"><span className="text-[var(--text-tertiary)]">Linked MR:</span> <a href={selectedIssue.linked_mr_url} target="_blank" rel="noopener noreferrer" className="text-[var(--accent)] ml-1 hover:underline">View MR</a></div>}
              </div>

              {/* Labels */}
              {selectedIssue.labels && selectedIssue.labels.length > 0 && (
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] mb-1.5">Labels</div>
                  <div className="flex flex-wrap gap-1.5">
                    {selectedIssue.labels.map(l => (
                      <span key={l} className="text-[10px] px-2 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)] border border-[var(--border)]">{l}</span>
                    ))}
                  </div>
                </div>
              )}

              {/* Description */}
              {selectedIssue.description && (
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] mb-1.5">Description</div>
                  <div className="text-sm text-[var(--text-primary)] bg-[var(--bg)] rounded-lg p-3 whitespace-pre-wrap">{selectedIssue.description}</div>
                </div>
              )}

              {/* Transitions */}
              {transitions.length > 0 && (
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] mb-2">Transitions</div>
                  <div className="flex flex-wrap gap-2">
                    {transitions.map(t => (
                      <button key={t.id} onClick={() => handleTransition(t.to)} className={`text-xs px-3 py-1.5 rounded-lg border transition-colors ${t.to === 'Done' || t.to === 'Closed' ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20 hover:bg-emerald-500/15' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--bg)]'}`}>
                        {t.name}
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {/* Comments */}
              <div>
                <div className="text-xs text-[var(--text-tertiary)] mb-2">Comments ({comments.length})</div>
                <div className="space-y-3 max-h-64 overflow-y-auto">
                  {comments.length === 0 && <div className="text-xs text-[var(--text-tertiary)] italic">No comments yet</div>}
                  {comments.map(c => (
                    <div key={c.id} className="bg-[var(--bg)] rounded-lg p-3">
                      <div className="flex items-center justify-between mb-1">
                        <span className="text-xs font-medium text-[var(--text-primary)]">{c.author || 'Unknown'}</span>
                        <span className="text-[10px] text-[var(--text-tertiary)]">{new Date(c.created_at).toLocaleString()}</span>
                      </div>
                      <div className="text-xs text-[var(--text-secondary)] whitespace-pre-wrap">{c.body}</div>
                    </div>
                  ))}
                </div>
                <div className="mt-3 flex gap-2">
                  <input
                    type="text"
                    value={commentText}
                    onChange={e => setCommentText(e.target.value)}
                    onKeyDown={e => e.key === 'Enter' && handleAddComment()}
                    placeholder="Add a comment..."
                    className="flex-1 text-xs border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] text-[var(--text-primary)]"
                  />
                  <button onClick={handleAddComment} disabled={!commentText.trim()} className="px-3 py-2 bg-[var(--accent)] text-white text-xs rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity">
                    Send
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Create Issue Modal */}
      {createOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => setCreateOpen(false)} />
          <div className="relative bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] w-full max-w-md mx-4 overflow-hidden">
            <div className="px-6 py-4 border-b border-[var(--border-light)]">
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">Create Jira Issue</h3>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Project Key *</label>
                <input value={newIssue.project_key} onChange={e => setNewIssue(n => ({ ...n, project_key: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="e.g. PEPA" />
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Summary *</label>
                <input value={newIssue.summary} onChange={e => setNewIssue(n => ({ ...n, summary: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="Issue summary" />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Type</label>
                  <select value={newIssue.issue_type} onChange={e => setNewIssue(n => ({ ...n, issue_type: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    {['Task', 'Story', 'Bug', 'Epic', 'Sub-task', 'Spike'].map(t => <option key={t} value={t}>{t}</option>)}
                  </select>
                </div>
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Priority</label>
                  <select value={newIssue.priority} onChange={e => setNewIssue(n => ({ ...n, priority: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    {['Critical', 'High', 'Medium', 'Low', 'Lowest'].map(p => <option key={p} value={p}>{p}</option>)}
                  </select>
                </div>
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Description</label>
                <textarea value={newIssue.description} onChange={e => setNewIssue(n => ({ ...n, description: e.target.value }))} rows={3} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] resize-none" placeholder="Issue description" />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Assignee</label>
                  <input value={newIssue.assignee} onChange={e => setNewIssue(n => ({ ...n, assignee: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="Username" />
                </div>
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Labels (comma-separated)</label>
                  <input value={newIssue.labels} onChange={e => setNewIssue(n => ({ ...n, labels: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="frontend, bug" />
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2 px-6 py-3 bg-[var(--bg)] border-t border-[var(--border-light)]">
              <button onClick={() => setCreateOpen(false)} className="btn btn-secondary flex-1 justify-center">Cancel</button>
              <button onClick={handleCreateIssue} disabled={!newIssue.summary || !newIssue.project_key} className="btn btn-primary flex-1 justify-center">Create Issue</button>
            </div>
          </div>
        </div>
      )}

      {/* Automation Rule Modal */}
      {ruleModalOpen && editingRule && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => { setRuleModalOpen(false); setEditingRule(null); }} />
          <div className="relative bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] w-full max-w-md mx-4 overflow-hidden">
            <div className="px-6 py-4 border-b border-[var(--border-light)]">
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">{editingRule.id ? 'Edit' : 'Create'} Automation Rule</h3>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Rule Name *</label>
                <input value={editingRule.name || ''} onChange={e => setEditingRule(r => r ? { ...r, name: e.target.value } : r)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="e.g. Auto-comment on deployment" />
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Description</label>
                <input value={editingRule.description || ''} onChange={e => setEditingRule(r => r ? { ...r, description: e.target.value } : r)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="What this rule does" />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Trigger *</label>
                  <select value={editingRule.trigger_type || ''} onChange={e => setEditingRule(r => r ? { ...r, trigger_type: e.target.value } : r)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    {triggerTypes.map(t => <option key={t.value} value={t.value}>{t.label}</option>)}
                  </select>
                </div>
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Action *</label>
                  <select value={editingRule.action_type || ''} onChange={e => setEditingRule(r => r ? { ...r, action_type: e.target.value } : r)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    {actionTypes.map(a => <option key={a.value} value={a.value}>{a.label}</option>)}
                  </select>
                </div>
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Jira Project Key</label>
                <input value={editingRule.jira_project_key || ''} onChange={e => setEditingRule(r => r ? { ...r, jira_project_key: e.target.value } : r)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="e.g. PEPA" />
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">JQL Filter (optional)</label>
                <input value={editingRule.jql_filter || ''} onChange={e => setEditingRule(r => r ? { ...r, jql_filter: e.target.value } : r)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="e.g. status = 'In Review'" />
              </div>
              <div className="flex items-center gap-2">
                <input type="checkbox" checked={editingRule.enabled ?? true} onChange={e => setEditingRule(r => r ? { ...r, enabled: e.target.checked } : r)} id="rule-enabled" />
                <label htmlFor="rule-enabled" className="text-xs text-[var(--text-secondary)]">Enabled</label>
              </div>
            </div>
            <div className="flex items-center gap-2 px-6 py-3 bg-[var(--bg)] border-t border-[var(--border-light)]">
              <button onClick={() => { setRuleModalOpen(false); setEditingRule(null); }} className="btn btn-secondary flex-1 justify-center">Cancel</button>
              <button onClick={handleSaveRule} disabled={!editingRule.name || !editingRule.trigger_type || !editingRule.action_type} className="btn btn-primary flex-1 justify-center">Save Rule</button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Rule Confirmation */}
      <ConfirmModal
        open={!!deleteRuleConfirm}
        title="Delete Automation Rule"
        description="Are you sure you want to delete this rule? This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        onConfirm={handleDeleteRule}
        onCancel={() => setDeleteRuleConfirm(null)}
      />
      </div>
    </div>
  );
}
