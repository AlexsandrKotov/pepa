'use client';
import { useState, useEffect, useCallback, useMemo } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { jira, plugins, type JiraIssue, type JiraComment, type JiraAutomationRule, type JiraFilters, type JiraStats, type JiraAssignee, type JiraSprint, type JiraWorklog, type JiraIssueLink } from '@/lib/api';
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

const boardColumns = ['Open', 'To Do', 'In Progress', 'In Review', 'Done', 'Closed'];

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
  const [assignees, setAssignees] = useState<JiraAssignee[]>([]);
  const [sprints, setSprints] = useState<JiraSprint[]>([]);
  const [components, setComponents] = useState<string[]>([]);
  const [projects, setProjects] = useState<{ key: string }[]>([]);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [syncing, setSyncing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'issues' | 'board' | 'my-tasks' | 'automation'>('issues');

  // Filters
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [filters, setFilters] = useState<JiraFilters>({});
  const [searchText, setSearchText] = useState('');

  // Issue detail
  const [selectedIssue, setSelectedIssue] = useState<JiraIssue | null>(null);
  const [comments, setComments] = useState<JiraComment[]>([]);
  const [commentText, setCommentText] = useState('');
  const [transitions, setTransitions] = useState<{ id: string; name: string; to: string }[]>([]);
  const [worklogs, setWorklogs] = useState<JiraWorklog[]>([]);
  const [totalTimeSpent, setTotalTimeSpent] = useState(0);
  const [issueLinks, setIssueLinks] = useState<JiraIssueLink[]>([]);
  const [worklogText, setWorklogText] = useState('');
  const [worklogTime, setWorklogTime] = useState('');

  // Create issue
  const [createOpen, setCreateOpen] = useState(false);
  const [createDropdownOpen, setCreateDropdownOpen] = useState(false);
  const [newIssue, setNewIssue] = useState({
    summary: '', project_key: '', issue_type: 'Task', description: '', priority: 'Medium',
    assignee: '', labels: '' as string,
    parent_key: '', epic_link: '', linked_issue_key: '', link_type: 'Relates',
  });
  const [creating, setCreating] = useState(false);

  // Edit issue
  const [editOpen, setEditOpen] = useState(false);
  const [editIssue, setEditIssue] = useState<Partial<JiraIssue> | null>(null);

  // Automation
  const [rules, setRules] = useState<JiraAutomationRule[]>([]);
  const [ruleModalOpen, setRuleModalOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<Partial<JiraAutomationRule> | null>(null);
  const [deleteRuleConfirm, setDeleteRuleConfirm] = useState<string | null>(null);

  // Escape key closes modals/panels
  const anyModalOpen = selectedIssue !== null || createOpen || createDropdownOpen || editOpen || ruleModalOpen || deleteRuleConfirm !== null;
  useEscapeKey(() => {
    if (deleteRuleConfirm) setDeleteRuleConfirm(null);
    else if (ruleModalOpen) { setRuleModalOpen(false); setEditingRule(null); }
    else if (editOpen) { setEditOpen(false); setEditIssue(null); }
    else if (createOpen) setCreateOpen(false);
    else if (createDropdownOpen) setCreateDropdownOpen(false);
    else if (selectedIssue) setSelectedIssue(null);
  }, anyModalOpen);

  // Check if Jira plugin is installed
  useEffect(() => {
    plugins.list()
      .then(d => {
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
    jira.getAssignees().then(d => setAssignees(d.assignees || [])).catch(() => {});
    jira.getSprints().then(d => setSprints(d.sprints || [])).catch(() => {});
    jira.getComponents().then(d => setComponents(d.components || [])).catch(() => {});
    jira.getProjects().then(d => setProjects(d.projects || [])).catch(() => {});
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
      const [commentsData, transData, worklogsData, linksData] = await Promise.all([
        jira.getComments(issue.id),
        jira.getTransitions(issue.id),
        jira.getWorklogs(issue.id).catch(() => ({ worklogs: [], total_seconds: 0 })),
        jira.getIssueLinks(issue.id).catch(() => ({ links: [] })),
      ]);
      setComments(commentsData.comments || []);
      setTransitions(transData.transitions || []);
      setWorklogs(worklogsData.worklogs || []);
      setTotalTimeSpent(worklogsData.total_seconds || 0);
      setIssueLinks(linksData.links || []);
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

  const handleInlineTransition = async (issue: JiraIssue, status: string, e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await jira.transition(issue.id, status);
      setToast({ message: `${issue.issue_key} → ${status}`, type: 'success' });
      await refresh();
    } catch {
      setToast({ message: 'Failed to transition', type: 'error' });
    }
  };

  const handleAddWorklog = async () => {
    if (!selectedIssue || !worklogTime.trim()) return;
    try {
      await jira.addWorklog(selectedIssue.id, { time_spent: worklogTime, comment: worklogText || undefined });
      setWorklogTime('');
      setWorklogText('');
      const data = await jira.getWorklogs(selectedIssue.id);
      setWorklogs(data.worklogs || []);
      setTotalTimeSpent(data.total_seconds || 0);
      setToast({ message: 'Worklog added', type: 'success' });
    } catch {
      setToast({ message: 'Failed to add worklog', type: 'error' });
    }
  };

  const handleCreateIssue = async () => {
    if (!newIssue.summary || !newIssue.project_key) return;
    setCreating(true);
    try {
      const result = await jira.createInJira({
        summary: newIssue.summary,
        project_key: newIssue.project_key,
        issue_type: newIssue.issue_type,
        description: newIssue.description || undefined,
        priority: newIssue.priority || undefined,
        assignee: newIssue.assignee || undefined,
        labels: newIssue.labels ? newIssue.labels.split(',').map(l => l.trim()).filter(Boolean) : undefined,
        parent_key: newIssue.issue_type === 'Sub-task' ? newIssue.parent_key : undefined,
        epic_link: newIssue.epic_link || undefined,
        linked_issue_key: newIssue.linked_issue_key || undefined,
        link_type: newIssue.linked_issue_key ? newIssue.link_type : undefined,
      });
      setToast({ message: `${result.issue_key} created in Jira`, type: 'success' });
      setCreateOpen(false);
      setCreateDropdownOpen(false);
      resetCreateForm();
      await refresh();
    } catch {
      setToast({ message: 'Failed to create in Jira', type: 'error' });
    }
    setCreating(false);
  };

  const resetCreateForm = () => {
    setNewIssue({
      summary: '', project_key: '', issue_type: 'Task', description: '', priority: 'Medium',
      assignee: '', labels: '', parent_key: '', epic_link: '', linked_issue_key: '', link_type: 'Relates',
    });
  };

  const openCreateModal = (type: string) => {
    setNewIssue(n => ({ ...n, issue_type: type }));
    setCreateDropdownOpen(false);
    setCreateOpen(true);
  };

  const openEditModal = (issue: JiraIssue) => {
    setEditIssue({ ...issue });
    setEditOpen(true);
  };

  const handleUpdateIssue = async () => {
    if (!editIssue?.id) return;
    try {
      await jira.update(editIssue.id, {
        summary: editIssue.summary,
        description: editIssue.description,
        assignee: editIssue.assignee,
        priority: editIssue.priority,
        labels: editIssue.labels,
      });
      setToast({ message: 'Issue updated', type: 'success' });
      setEditOpen(false);
      setEditIssue(null);
      await refresh();
      if (selectedIssue?.id === editIssue.id) {
        openIssueDetail({ ...selectedIssue, ...editIssue } as JiraIssue);
      }
    } catch {
      setToast({ message: 'Failed to update issue', type: 'error' });
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
    filters.components?.length,
  ].filter(Boolean).reduce((a, b) => (a || 0) + (b || 0), 0) || 0;

  // Board view: group issues by status
  const boardData = useMemo(() => {
    const grouped: Record<string, JiraIssue[]> = {};
    boardColumns.forEach(col => { grouped[col] = []; });
    issues.forEach(issue => {
      const col = boardColumns.find(c => c === issue.status) || 'Open';
      if (!grouped[col]) grouped[col] = [];
      grouped[col].push(issue);
    });
    return grouped;
  }, [issues]);

  // My tasks: issues assigned to current user (from assignees list)
  const [myIssues, setMyIssues] = useState<JiraIssue[]>([]);
  const [myIssuesTotal, setMyIssuesTotal] = useState(0);
  const [myTasksAssignee, setMyTasksAssignee] = useState('');

  const loadMyIssues = useCallback(async () => {
    if (!myTasksAssignee) return;
    try {
      const data = await jira.getMyIssues(myTasksAssignee);
      setMyIssues(data.issues || []);
      setMyIssuesTotal(data.total || 0);
    } catch { /* ignore */ }
  }, [myTasksAssignee]);

  useEffect(() => { if (activeTab === 'my-tasks') loadMyIssues(); }, [activeTab, loadMyIssues]);

  const formatTimeSpent = (secs: number) => {
    if (secs === 0) return '0m';
    const h = Math.floor(secs / 3600);
    const m = Math.floor((secs % 3600) / 60);
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  };

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
            <div className="text-4xl mb-3 opacity-30">{'\u{1F50C}'}</div>
            <h3 className="text-[15px] font-medium text-[var(--text-primary)] mb-2">Jira plugin is not installed</h3>
            <p className="text-[13px] text-[var(--text-secondary)] mb-4">
              Install the Jira plugin from the Marketplace to manage issues, automate workflows, and track deployments.
            </p>
            <a href="/marketplace" className="btn btn-primary">Go to Marketplace</a>
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
      <div className="grid grid-cols-6 gap-4 page-animate">
        {[
          { label: 'Total Issues', value: stats?.total ?? total, color: 'text-blue-400' },
          { label: 'In Progress', value: (stats?.by_status?.['In Progress'] || 0) + (stats?.by_status?.['In Review'] || 0), color: 'text-amber-400' },
          { label: 'Done', value: stats?.by_status?.['Done'] || 0, color: 'text-emerald-400' },
          { label: 'Open Bugs', value: stats?.open_bugs || 0, color: 'text-red-400' },
          { label: 'Closed', value: (stats?.by_status?.['Closed'] || 0) + (stats?.by_status?.['Done'] || 0), color: 'text-gray-400' },
          { label: 'Sprints Active', value: sprints.filter(s => s.state === 'active').length, color: 'text-violet-400' },
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
            {[
              { key: 'issues', label: 'Issues' },
              { key: 'board', label: 'Board' },
              { key: 'my-tasks', label: 'My Tasks' },
              { key: 'automation', label: 'Automation' },
            ].map(tab => (
              <button key={tab.key} onClick={() => setActiveTab(tab.key as typeof activeTab)} className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${activeTab === tab.key ? 'bg-[var(--surface)] text-[var(--text-primary)] shadow-sm' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}`}>
                {tab.label}
              </button>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <div className="relative">
            <button onClick={() => setCreateDropdownOpen(!createDropdownOpen)} className="px-3 py-1.5 bg-[var(--surface)] border border-[var(--border)] text-[var(--text-primary)] text-sm rounded-lg hover:bg-[var(--bg)] flex items-center gap-1.5">
              + Create
              <svg className="w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" /></svg>
            </button>
            {createDropdownOpen && (
              <>
                <div className="fixed inset-0 z-40" onClick={() => setCreateDropdownOpen(false)} />
                <div className="absolute right-0 mt-1 w-48 bg-[var(--surface)] border border-[var(--border)] rounded-lg shadow-xl z-50 py-1 overflow-hidden">
                  {[
                    { type: 'Task', icon: '\u2705', label: 'Task' },
                    { type: 'Story', icon: '\u{1F4D6}', label: 'Story' },
                    { type: 'Bug', icon: '\u{1F41B}', label: 'Bug' },
                    { type: 'Epic', icon: '\u{1F3D4}', label: 'Epic' },
                    { type: 'Sub-task', icon: '\u{1F517}', label: 'Sub-task' },
                  ].map(item => (
                    <button key={item.type} onClick={() => openCreateModal(item.type)}
                      className="w-full text-left px-3 py-2 text-sm text-[var(--text-primary)] hover:bg-[var(--bg)] flex items-center gap-2 transition-colors">
                      <span className="text-sm">{item.icon}</span>
                      <span>{item.label}</span>
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>
          <button onClick={handleSync} disabled={syncing} className="px-3 py-1.5 bg-[var(--accent)] text-white text-sm rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity">
            {syncing ? 'Syncing...' : 'Sync from Jira'}
          </button>
        </div>
      </div>

      {/* ── Issues Tab ──────────────────────────────────── */}
      {activeTab === 'issues' && (
        <>
          {/* Search + Filter Toggle */}
          <div className="flex items-center gap-3 page-animate-up page-delay-1">
            <div className="flex-1 flex items-center gap-2 bg-[var(--surface)] border border-[var(--border)] rounded-lg px-3 py-2">
              <svg className="w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" /></svg>
              <input type="text" placeholder="Search issues by key, summary, or description..." value={searchText}
                onChange={e => setSearchText(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleSearch()}
                className="flex-1 text-sm text-[var(--text-primary)] placeholder-[#a3a3a3] outline-none bg-transparent" />
              {searchText && (
                <button onClick={() => { setSearchText(''); setFilters(f => ({ ...f, search: undefined })); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
                </button>
              )}
            </div>
            <button onClick={() => setFiltersOpen(!filtersOpen)}
              className={`flex items-center gap-2 px-3 py-2 border rounded-lg text-sm transition-colors ${filtersOpen || activeFilterCount > 0 ? 'border-blue-500/20 bg-blue-500/10 text-[var(--accent)]' : 'border-[var(--border)] bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--bg)]'}`}>
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" /></svg>
              Filters{activeFilterCount > 0 && <span className="bg-[var(--accent)] text-white text-[10px] px-1.5 py-0.5 rounded-full">{activeFilterCount}</span>}
            </button>
            {activeFilterCount > 0 && <button onClick={clearFilters} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">Clear all</button>}
          </div>

          {/* Collapsible Filter Panel */}
          {filtersOpen && (
            <div className="bg-[var(--surface)] rounded-xl border border-[var(--border)] p-4 space-y-4 page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
              <div className="grid grid-cols-3 gap-4">
                {/* Project */}
                {projects.length > 0 && (
                  <div>
                    <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Project</div>
                    <select value={filters.project_key || ''} onChange={e => setFilters(f => ({ ...f, project_key: e.target.value || undefined }))}
                      className="w-full text-xs border border-[var(--border)] rounded-lg px-2 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]">
                      <option value="">All Projects</option>
                      {projects.map(p => <option key={p.key} value={p.key}>{p.key}</option>)}
                    </select>
                  </div>
                )}
                {/* Assignee */}
                <div>
                  <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Assignee</div>
                  <select value={filters.assignee || ''} onChange={e => setFilters(f => ({ ...f, assignee: e.target.value || undefined }))}
                    className="w-full text-xs border border-[var(--border)] rounded-lg px-2 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]">
                    <option value="">All Assignees</option>
                    {assignees.map(a => <option key={a.jira_account} value={a.display_name}>{a.display_name}</option>)}
                  </select>
                </div>
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
              </div>
              <div className="grid grid-cols-3 gap-4">
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
                {/* Components */}
                {components.length > 0 && (
                  <div>
                    <div className="text-xs font-medium text-[var(--text-tertiary)] mb-2">Component</div>
                    <div className="flex flex-wrap gap-1.5">
                      {components.map(c => (
                        <button key={c} onClick={() => toggleFilter('components', c)} className={`text-xs px-2 py-1 rounded-md border transition-colors ${filters.components?.includes(c) ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--bg)]'}`}>
                          {c}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
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
          ) : issues.length === 0 ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center page-animate-up page-delay-2">
              <div className="text-[var(--text-tertiary)] text-sm">
                {total === 0 ? 'No Jira issues synced yet. Click "Sync from Jira" to pull issues.' : 'No issues match the selected filters.'}
              </div>
              {activeFilterCount > 0 && <button onClick={clearFilters} className="mt-2 text-xs text-[var(--accent)] hover:underline">Clear filters</button>}
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
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border-light)]">
                  {issues.map(issue => (
                    <tr key={issue.id} onClick={() => openIssueDetail(issue)} className="hover:bg-[var(--bg)] cursor-pointer">
                      <td className="px-4 py-3"><span className="font-mono text-xs font-medium text-[var(--accent)]">{issue.issue_key}</span></td>
                      <td className="px-4 py-3"><div className="text-[13px] text-[var(--text-primary)] truncate max-w-[300px]">{issue.summary}</div></td>
                      <td className="px-4 py-3"><span className="text-xs">{typeIcons[issue.issue_type] || '\u{1F4C4}'} {issue.issue_type}</span></td>
                      <td className="px-4 py-3"><span className={`text-xs px-2 py-0.5 rounded-full border ${priorityColors[issue.priority] || 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]'}`}>{issue.priority}</span></td>
                      <td className="px-4 py-3"><span className={`text-xs px-2 py-0.5 rounded-full ${statusColors[issue.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>{issue.status}</span></td>
                      <td className="px-4 py-3 text-xs text-[var(--text-secondary)]">{issue.assignee || '-'}</td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1 flex-wrap">
                          {(issue.labels || []).slice(0, 3).map(l => (
                            <span key={l} className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)]">{l}</span>
                          ))}
                          {(issue.labels || []).length > 3 && <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-tertiary)]">+{issue.labels.length - 3}</span>}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1" onClick={e => e.stopPropagation()}>
                          {issue.status !== 'Done' && issue.status !== 'Closed' && (
                            <button onClick={e => handleInlineTransition(issue, 'In Progress', e)} className="text-[10px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500 hover:bg-blue-500/20" title="Start Progress">Start</button>
                          )}
                          {issue.status !== 'Done' && issue.status !== 'Closed' && (
                            <button onClick={e => handleInlineTransition(issue, 'Done', e)} className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20" title="Mark Done">Done</button>
                          )}
                          <button onClick={() => openEditModal(issue)} className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)] hover:bg-[var(--border)]" title="Edit">Edit</button>
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

      {/* ── Board Tab ───────────────────────────────────── */}
      {activeTab === 'board' && (
        <div className="page-animate-up page-delay-1">
          {/* Active sprints */}
          {sprints.filter(s => s.state === 'active').length > 0 && (
            <div className="mb-4 flex items-center gap-3">
              <span className="text-xs text-[var(--text-tertiary)]">Active Sprint:</span>
              {sprints.filter(s => s.state === 'active').map(s => (
                <span key={s.id} className="text-xs px-2 py-1 rounded-lg bg-violet-500/10 text-violet-500 border border-violet-500/20">{s.name}</span>
              ))}
            </div>
          )}
          <div className="grid grid-cols-6 gap-3 overflow-x-auto">
            {boardColumns.map(col => (
              <div key={col} className="min-w-[200px]">
                <div className="flex items-center justify-between mb-2 px-1">
                  <span className={`text-xs font-medium px-2 py-0.5 rounded-full ${statusColors[col] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>{col}</span>
                  <span className="text-[10px] text-[var(--text-tertiary)]">{(boardData[col] || []).length}</span>
                </div>
                <div className="space-y-2">
                  {(boardData[col] || []).map(issue => (
                    <div key={issue.id} onClick={() => openIssueDetail(issue)}
                      className="bg-[var(--surface)] border border-[var(--border)] rounded-lg p-3 cursor-pointer hover:border-[var(--accent)]/30 transition-colors">
                      <div className="flex items-center gap-1.5 mb-1">
                        <span className="font-mono text-[10px] text-[var(--accent)]">{issue.issue_key}</span>
                        <span className="text-[10px]">{typeIcons[issue.issue_type] || '\u{1F4C4}'}</span>
                      </div>
                      <div className="text-xs text-[var(--text-primary)] line-clamp-2 mb-2">{issue.summary}</div>
                      <div className="flex items-center justify-between">
                        <span className={`text-[10px] px-1.5 py-0.5 rounded-full border ${priorityColors[issue.priority] || ''}`}>{issue.priority}</span>
                        <span className="text-[10px] text-[var(--text-tertiary)] truncate max-w-[80px]">{issue.assignee || 'Unassigned'}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ── My Tasks Tab ────────────────────────────────── */}
      {activeTab === 'my-tasks' && (
        <div className="space-y-4 page-animate-up page-delay-1">
          <div className="flex items-center gap-3">
            <select value={myTasksAssignee} onChange={e => setMyTasksAssignee(e.target.value)}
              className="text-sm border border-[var(--border)] rounded-lg px-3 py-2 bg-[var(--surface)] text-[var(--text-primary)]">
              <option value="">Select assignee...</option>
              {assignees.map(a => <option key={a.jira_account} value={a.display_name}>{a.display_name}</option>)}
            </select>
            {myTasksAssignee && <span className="text-xs text-[var(--text-tertiary)]">{myIssuesTotal} open issues</span>}
          </div>
          {!myTasksAssignee ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center">
              <div className="text-[var(--text-tertiary)] text-sm">Select an assignee to view their tasks</div>
            </div>
          ) : myIssues.length === 0 ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center">
              <div className="text-[var(--text-tertiary)] text-sm">No open issues assigned to {myTasksAssignee}</div>
            </div>
          ) : (
            <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] overflow-hidden">
              <table className="w-full text-sm">
                <thead className="bg-[var(--bg)] border-b border-[var(--border)]">
                  <tr>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Key</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Summary</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Type</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Priority</th>
                    <th className="text-left px-4 py-3 text-[12px] font-medium text-[var(--text-tertiary)]">Status</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border-light)]">
                  {myIssues.map(issue => (
                    <tr key={issue.id} onClick={() => openIssueDetail(issue)} className="hover:bg-[var(--bg)] cursor-pointer">
                      <td className="px-4 py-3"><span className="font-mono text-xs font-medium text-[var(--accent)]">{issue.issue_key}</span></td>
                      <td className="px-4 py-3"><div className="text-[13px] text-[var(--text-primary)] truncate max-w-[400px]">{issue.summary}</div></td>
                      <td className="px-4 py-3"><span className="text-xs">{typeIcons[issue.issue_type] || '\u{1F4C4}'} {issue.issue_type}</span></td>
                      <td className="px-4 py-3"><span className={`text-xs px-2 py-0.5 rounded-full border ${priorityColors[issue.priority] || ''}`}>{issue.priority}</span></td>
                      <td className="px-4 py-3"><span className={`text-xs px-2 py-0.5 rounded-full ${statusColors[issue.status] || ''}`}>{issue.status}</span></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* ── Automation Tab ──────────────────────────────── */}
      {activeTab === 'automation' && (
        <div className="space-y-4 page-animate-up page-delay-1">
          <div className="flex items-center justify-between">
            <p className="text-sm text-[var(--text-secondary)]">Automate Jira actions when PEPA events occur.</p>
            <button onClick={() => { setEditingRule({ name: '', description: '', trigger_type: 'deployment_succeeded', jira_project_key: '', jql_filter: '', action_type: 'add_comment', action_config: {}, enabled: true }); setRuleModalOpen(true); }} className="px-3 py-1.5 bg-[var(--accent)] text-white text-sm rounded-lg hover:opacity-90 transition-opacity">
              + New Rule
            </button>
          </div>
          {rules.length === 0 ? (
            <div className="rounded-xl border border-dashed border-[var(--border)] p-12 text-center">
              <div className="text-2xl mb-2">{'\u2699\uFE0F'}</div>
              <div className="text-[var(--text-tertiary)] text-sm">No automation rules yet.</div>
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
                    {rule.last_triggered_at && <span className="text-[10px] text-[var(--text-tertiary)]">Last: {new Date(rule.last_triggered_at).toLocaleDateString()}</span>}
                    <button onClick={() => { setEditingRule(rule); setRuleModalOpen(true); }} className="text-xs text-[var(--accent)] hover:underline">Edit</button>
                    <button onClick={() => setDeleteRuleConfirm(rule.id)} className="text-xs text-red-500 hover:underline">Delete</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ── Issue Detail Side Panel ─────────────────────── */}
      {selectedIssue && (
        <div className="fixed inset-0 z-50 flex">
          <div className="absolute inset-0 bg-black/20 backdrop-blur-sm" onClick={() => setSelectedIssue(null)} />
          <div className="relative ml-auto w-full max-w-lg bg-[var(--surface)] h-full overflow-y-auto shadow-2xl border-l border-[var(--border)]">
            <div className="sticky top-0 bg-[var(--surface)] border-b border-[var(--border)] px-6 py-4 flex items-center justify-between z-10">
              <div className="flex items-center gap-3">
                <span className="font-mono text-sm font-medium text-[var(--accent)]">{selectedIssue.issue_key}</span>
                <span className={`text-xs px-2 py-0.5 rounded-full ${statusColors[selectedIssue.status] || 'bg-[var(--border-light)] text-[var(--text-secondary)]'}`}>{selectedIssue.status}</span>
              </div>
              <div className="flex items-center gap-2">
                {selectedIssue.jira_url && <a href={selectedIssue.jira_url} target="_blank" rel="noopener noreferrer" className="text-xs text-[var(--accent)] hover:underline">Open in Jira</a>}
                <button onClick={() => { setSelectedIssue(null); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                  <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
                </button>
              </div>
            </div>

            <div className="p-6 space-y-6">
              {/* Summary & Type */}
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <span className="text-sm">{typeIcons[selectedIssue.issue_type] || '\u{1F4C4}'} {selectedIssue.issue_type}</span>
                  <span className={`text-xs px-2 py-0.5 rounded-full border ${priorityColors[selectedIssue.priority] || ''}`}>{selectedIssue.priority}</span>
                  <button onClick={() => openEditModal(selectedIssue)} className="ml-auto text-xs text-[var(--accent)] hover:underline">Edit</button>
                </div>
                <h2 className="text-base font-semibold text-[var(--text-primary)]">{selectedIssue.summary}</h2>
              </div>

              {/* Meta */}
              <div className="grid grid-cols-2 gap-3 text-xs">
                <div><span className="text-[var(--text-tertiary)]">Assignee:</span> <span className="text-[var(--text-primary)] ml-1">{selectedIssue.assignee || 'Unassigned'}</span></div>
                <div><span className="text-[var(--text-tertiary)]">Reporter:</span> <span className="text-[var(--text-primary)] ml-1">{selectedIssue.reporter || '-'}</span></div>
                <div><span className="text-[var(--text-tertiary)]">Project:</span> <span className="text-[var(--text-primary)] ml-1">{selectedIssue.project_key}</span></div>
                {selectedIssue.parent_key && <div><span className="text-[var(--text-tertiary)]">Parent:</span> <span className="text-[var(--accent)] ml-1">{selectedIssue.parent_key}</span></div>}
                {totalTimeSpent > 0 && <div><span className="text-[var(--text-tertiary)]">Time Spent:</span> <span className="text-[var(--text-primary)] ml-1">{formatTimeSpent(totalTimeSpent)}</span></div>}
              </div>

              {/* Labels */}
              {selectedIssue.labels && selectedIssue.labels.length > 0 && (
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] mb-1.5">Labels</div>
                  <div className="flex flex-wrap gap-1.5">
                    {selectedIssue.labels.map(l => <span key={l} className="text-[10px] px-2 py-0.5 rounded bg-[var(--bg)] text-[var(--text-secondary)] border border-[var(--border)]">{l}</span>)}
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

              {/* Issue Links */}
              {issueLinks.length > 0 && (
                <div>
                  <div className="text-xs text-[var(--text-tertiary)] mb-2">Linked Issues</div>
                  <div className="space-y-1">
                    {issueLinks.map(link => {
                      const isOutward = link.inward_key === selectedIssue.issue_key;
                      const otherKey = isOutward ? link.outward_key : link.inward_key;
                      const label = isOutward ? link.outward_label : link.inward_label;
                      return (
                        <div key={link.id} className="flex items-center gap-2 text-xs">
                          <span className="text-[var(--text-tertiary)]">{label || link.link_type}:</span>
                          <span className="text-[var(--accent)] font-mono">{otherKey}</span>
                        </div>
                      );
                    })}
                  </div>
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

              {/* Worklogs */}
              <div>
                <div className="text-xs text-[var(--text-tertiary)] mb-2">Time Tracking {totalTimeSpent > 0 && <span className="text-[var(--text-primary)]">({formatTimeSpent(totalTimeSpent)} total)</span>}</div>
                <div className="space-y-2 max-h-32 overflow-y-auto">
                  {worklogs.length === 0 && <div className="text-xs text-[var(--text-tertiary)] italic">No worklogs yet</div>}
                  {worklogs.map(w => (
                    <div key={w.id} className="bg-[var(--bg)] rounded-lg p-2 flex items-center justify-between">
                      <div>
                        <span className="text-xs font-medium text-[var(--text-primary)]">{w.author}</span>
                        <span className="text-[10px] text-[var(--text-tertiary)] ml-2">{new Date(w.started_at).toLocaleDateString()}</span>
                        {w.comment && <div className="text-[10px] text-[var(--text-secondary)] mt-0.5">{w.comment}</div>}
                      </div>
                      <span className="text-xs font-medium text-[var(--accent)]">{w.time_spent || formatTimeSpent(w.time_spent_secs)}</span>
                    </div>
                  ))}
                </div>
                <div className="mt-2 flex gap-2">
                  <input type="text" value={worklogTime} onChange={e => setWorklogTime(e.target.value)} placeholder="e.g. 2h 30m" className="w-24 text-xs border border-[var(--border)] rounded-lg px-2 py-1.5 outline-none focus:border-[var(--accent)] text-[var(--text-primary)]" />
                  <input type="text" value={worklogText} onChange={e => setWorklogText(e.target.value)} placeholder="Comment (optional)" className="flex-1 text-xs border border-[var(--border)] rounded-lg px-2 py-1.5 outline-none focus:border-[var(--accent)] text-[var(--text-primary)]" />
                  <button onClick={handleAddWorklog} disabled={!worklogTime.trim()} className="px-2 py-1.5 bg-[var(--accent)] text-white text-xs rounded-lg hover:opacity-90 disabled:opacity-50">Log</button>
                </div>
              </div>

              {/* Comments */}
              <div>
                <div className="text-xs text-[var(--text-tertiary)] mb-2">Comments ({comments.length})</div>
                <div className="space-y-3 max-h-48 overflow-y-auto">
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
                  <input type="text" value={commentText} onChange={e => setCommentText(e.target.value)} onKeyDown={e => e.key === 'Enter' && handleAddComment()}
                    placeholder="Add a comment..." className="flex-1 text-xs border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] text-[var(--text-primary)]" />
                  <button onClick={handleAddComment} disabled={!commentText.trim()} className="px-3 py-2 bg-[var(--accent)] text-white text-xs rounded-lg hover:opacity-90 disabled:opacity-50 transition-opacity">Send</button>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Create Modal ────────────────────────────────── */}
      {createOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => setCreateOpen(false)} />
          <div className="relative bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] w-full max-w-lg mx-4 overflow-hidden max-h-[90vh] flex flex-col">
            <div className="px-6 py-4 border-b border-[var(--border-light)] flex items-center justify-between shrink-0">
              <div className="flex items-center gap-2">
                <span className="text-base">{typeIcons[newIssue.issue_type] || '\u{1F4C4}'}</span>
                <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">Create {newIssue.issue_type}</h3>
              </div>
              <div className="flex items-center gap-1">
                {['Task', 'Story', 'Bug', 'Epic', 'Sub-task'].map(t => (
                  <button key={t} onClick={() => setNewIssue(n => ({ ...n, issue_type: t }))}
                    className={`text-[10px] px-2 py-1 rounded-md border transition-colors ${newIssue.issue_type === t ? 'bg-[var(--accent)] text-white border-[var(--accent)]' : 'bg-[var(--surface)] text-[var(--text-secondary)] border-[var(--border)] hover:bg-[var(--bg)]'}`}
                    title={t}>
                    {typeIcons[t]} {t}
                  </button>
                ))}
              </div>
            </div>
            <div className="p-6 space-y-4 overflow-y-auto">
              {/* Project */}
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Project Key *</label>
                <select value={newIssue.project_key} onChange={e => setNewIssue(n => ({ ...n, project_key: e.target.value }))}
                  className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                  <option value="">Select project...</option>
                  {projects.map(p => <option key={p.key} value={p.key}>{p.key}</option>)}
                </select>
              </div>
              {/* Summary */}
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Summary *</label>
                <input value={newIssue.summary} onChange={e => setNewIssue(n => ({ ...n, summary: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder={`${newIssue.issue_type} summary`} />
              </div>
              {/* Priority + Assignee */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Priority</label>
                  <select value={newIssue.priority} onChange={e => setNewIssue(n => ({ ...n, priority: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    {['Critical', 'High', 'Medium', 'Low', 'Lowest'].map(p => <option key={p} value={p}>{p}</option>)}
                  </select>
                </div>
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Assignee</label>
                  <select value={newIssue.assignee} onChange={e => setNewIssue(n => ({ ...n, assignee: e.target.value }))}
                    className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    <option value="">Unassigned</option>
                    {assignees.map(a => <option key={a.jira_account} value={a.jira_account}>{a.display_name}</option>)}
                  </select>
                </div>
              </div>
              {/* Description */}
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Description</label>
                <textarea value={newIssue.description} onChange={e => setNewIssue(n => ({ ...n, description: e.target.value }))} rows={3} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] resize-none" placeholder="Describe the issue..." />
              </div>
              {/* Labels */}
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Labels (comma-separated)</label>
                <input value={newIssue.labels} onChange={e => setNewIssue(n => ({ ...n, labels: e.target.value }))} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" placeholder="frontend, backend" />
              </div>

              {/* ── Sub-task: Parent Key ── */}
              {newIssue.issue_type === 'Sub-task' && (
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Parent Issue Key *</label>
                  <input value={newIssue.parent_key} onChange={e => setNewIssue(n => ({ ...n, parent_key: e.target.value }))}
                    className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] font-mono"
                    placeholder="e.g. PROJ-123" />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">The parent issue this sub-task belongs to</p>
                </div>
              )}

              {/* ── Epic/Story/Task/Bug: Link to Epic ── */}
              {newIssue.issue_type !== 'Sub-task' && newIssue.issue_type !== 'Epic' && (
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Link to Epic (optional)</label>
                  <input value={newIssue.epic_link} onChange={e => setNewIssue(n => ({ ...n, epic_link: e.target.value }))}
                    className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] font-mono"
                    placeholder="e.g. PROJ-100" />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Epic issue key to associate this {newIssue.issue_type.toLowerCase()} with</p>
                </div>
              )}

              {/* ── Link to another issue ── */}
              <div className="border-t border-[var(--border-light)] pt-4">
                <div className="flex items-center gap-2 mb-3">
                  <svg className="w-3.5 h-3.5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758 4.829a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" /></svg>
                  <span className="text-xs font-medium text-[var(--text-secondary)]">Link to another issue</span>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-[10px] text-[var(--text-tertiary)] mb-1 block">Issue Key</label>
                    <input value={newIssue.linked_issue_key} onChange={e => setNewIssue(n => ({ ...n, linked_issue_key: e.target.value }))}
                      className="w-full text-xs border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] font-mono"
                      placeholder="e.g. PROJ-456" />
                  </div>
                  <div>
                    <label className="text-[10px] text-[var(--text-tertiary)] mb-1 block">Link Type</label>
                    <select value={newIssue.link_type} onChange={e => setNewIssue(n => ({ ...n, link_type: e.target.value }))}
                      className="w-full text-xs border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                      <option value="Relates">Relates to</option>
                      <option value="Blocks">Blocks</option>
                      <option value="Clones">Clones</option>
                      <option value="Duplicate">Duplicate of</option>
                    </select>
                  </div>
                </div>
              </div>
            </div>
            <div className="flex items-center gap-2 px-6 py-3 bg-[var(--bg)] border-t border-[var(--border-light)] shrink-0">
              <button onClick={() => { setCreateOpen(false); resetCreateForm(); }} className="btn btn-secondary flex-1 justify-center">Cancel</button>
              <button onClick={handleCreateIssue} disabled={!newIssue.summary || !newIssue.project_key || creating || (newIssue.issue_type === 'Sub-task' && !newIssue.parent_key)} className="btn btn-primary flex-1 justify-center">
                {creating ? 'Creating...' : `Create ${newIssue.issue_type}`}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Edit Issue Modal ────────────────────────────── */}
      {editOpen && editIssue && (
        <div className="fixed inset-0 z-50 flex items-center justify-center">
          <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={() => { setEditOpen(false); setEditIssue(null); }} />
          <div className="relative bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] w-full max-w-md mx-4 overflow-hidden">
            <div className="px-6 py-4 border-b border-[var(--border-light)]">
              <h3 className="text-[15px] font-semibold text-[var(--text-primary)]">Edit {editIssue.issue_key}</h3>
            </div>
            <div className="p-6 space-y-4">
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Summary</label>
                <input value={editIssue.summary || ''} onChange={e => setEditIssue(n => n ? { ...n, summary: e.target.value } : n)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Priority</label>
                  <select value={editIssue.priority || ''} onChange={e => setEditIssue(n => n ? { ...n, priority: e.target.value } : n)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    {['Critical', 'High', 'Medium', 'Low', 'Lowest'].map(p => <option key={p} value={p}>{p}</option>)}
                  </select>
                </div>
                <div>
                  <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Assignee</label>
                  <select value={editIssue.assignee || ''} onChange={e => setEditIssue(n => n ? { ...n, assignee: e.target.value } : n)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] bg-[var(--surface)]">
                    <option value="">Unassigned</option>
                    {assignees.map(a => <option key={a.jira_account} value={a.display_name}>{a.display_name}</option>)}
                  </select>
                </div>
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Description</label>
                <textarea value={editIssue.description || ''} onChange={e => setEditIssue(n => n ? { ...n, description: e.target.value } : n)} rows={3} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)] resize-none" />
              </div>
              <div>
                <label className="text-xs text-[var(--text-tertiary)] mb-1 block">Labels (comma-separated)</label>
                <input value={(editIssue.labels || []).join(', ')} onChange={e => setEditIssue(n => n ? { ...n, labels: e.target.value.split(',').map(l => l.trim()) } : n)} className="w-full text-sm border border-[var(--border)] rounded-lg px-3 py-2 outline-none focus:border-[var(--accent)]" />
              </div>
            </div>
            <div className="flex items-center gap-2 px-6 py-3 bg-[var(--bg)] border-t border-[var(--border-light)]">
              <button onClick={() => { setEditOpen(false); setEditIssue(null); }} className="btn btn-secondary flex-1 justify-center">Cancel</button>
              <button onClick={handleUpdateIssue} className="btn btn-primary flex-1 justify-center">Save Changes</button>
            </div>
          </div>
        </div>
      )}

      {/* ── Automation Rule Modal ───────────────────────── */}
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
      <ConfirmModal open={!!deleteRuleConfirm} title="Delete Automation Rule" description="Are you sure you want to delete this rule? This action cannot be undone."
        confirmLabel="Delete" variant="danger" onConfirm={handleDeleteRule} onCancel={() => setDeleteRuleConfirm(null)} />
      </div>
    </div>
  );
}
