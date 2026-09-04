'use client';

import { useState, useEffect, useCallback, Fragment } from 'react';
import { securityScan, devops, connections, registryRepositories, type ScanTarget, type ScanRun, type ScanSchedule, type SecurityDashboard, type ScannerType, type TargetType, type CompliancePolicy, type SecurityFinding, type SecurityFindingSummary, type RegistryRepository, type Connection } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import BrandIcon from '@/components/BrandIcon';
import ConfirmModal from '@/components/ConfirmModal';

type TabKey = 'overview' | 'targets' | 'scans' | 'reports' | 'schedules' | 'compliance' | 'findings';

const TABS: { key: TabKey; label: string; icon: string }[] = [
  { key: 'overview', label: 'Overview', icon: 'dashboard' },
  { key: 'targets', label: 'Scan Targets', icon: 'trivy' },
  { key: 'scans', label: 'Scan Results', icon: 'cicd' },
  { key: 'compliance', label: 'Compliance', icon: 'vault' },
  { key: 'findings', label: 'Findings', icon: 'discovery' },
  { key: 'reports', label: 'Reports', icon: 'sonarqube' },
  { key: 'schedules', label: 'Schedules', icon: 'prometheus' },
];

const SCANNER_COLORS: Record<string, string> = {
  trivy: '#0080FF',
  sonarqube: '#4E9BCD',
  both: '#8B5CF6',
};

const STATUS_COLORS: Record<string, string> = {
  completed: 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20',
  failed: 'bg-red-500/10 text-red-500 border-red-500/20',
  running: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
  pending: 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]',
  cancelled: 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]',
};

const SEVERITY_COLORS: Record<string, string> = {
  critical: '#dc2626',
  high: '#ea580c',
  medium: '#ca8a04',
  low: '#16a34a',
  unknown: '#6b7280',
};

export default function SecurityClient() {
  const [activeTab, setActiveTab] = useState<TabKey>('overview');
  const [dashboard, setDashboard] = useState<SecurityDashboard | null>(null);
  const [targets, setTargets] = useState<ScanTarget[]>([]);
  const [scans, setScans] = useState<ScanRun[]>([]);
  const [schedules, setSchedules] = useState<ScanSchedule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Compliance & Findings state
  const [policies, setPolicies] = useState<CompliancePolicy[]>([]);
  const [findings, setFindings] = useState<SecurityFinding[]>([]);
  const [findingSummary, setFindingSummary] = useState<SecurityFindingSummary | null>(null);
  const [showPolicyForm, setShowPolicyForm] = useState(false);
  const [policyForm, setPolicyForm] = useState({ name: '', description: '', policy_type: 'resource_limits' as CompliancePolicy['policy_type'], environment: 'production', severity: 'high' as CompliancePolicy['severity'], blocking: true, enabled: true });

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [dash, tgts, scns, scheds, pols, finds, fSummary] = await Promise.all([
        securityScan.getDashboard(),
        securityScan.listTargets(),
        securityScan.listScans({ limit: 50 }),
        securityScan.listSchedules(),
        devops.listPolicies().catch(() => []),
        devops.listFindings().catch(() => []),
        devops.getFindingSummary().catch(() => null),
      ]);
      setDashboard(dash);
      setTargets(tgts || []);
      setScans(scns || []);
      setSchedules(scheds || []);
      setPolicies(pols || []);
      setFindings(finds || []);
      setFindingSummary(fSummary);
    } catch (e) {
      setError(friendlyError(e).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {/* Header */}
      <div className="page-animate flex items-center justify-between">
        <div>
          <h1 className="page-title-modern">Security Scanning</h1>
          <p className="page-subtitle-modern">Manage vulnerability scans with Trivy and SonarQube</p>
        </div>
        <button
          onClick={() => securityScan.scanAll()}
          className="btn btn-primary"
        >
          Scan All
        </button>
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-[var(--border)]">
        {TABS.map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2 text-sm font-medium rounded-t-lg transition-colors ${
              activeTab === tab.key
                ? 'bg-[var(--bg-secondary)] text-[var(--text-primary)] border-b-2 border-blue-500'
                : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-secondary)]/50'
            }`}
          >
            <span className="mr-2"><BrandIcon name={tab.icon} size={16} /></span>
            {tab.label}
          </button>
        ))}
      </div>

      {error && (
        <div className="p-4 bg-red-500/10 border border-red-500/20 rounded-lg text-red-500 text-sm">
          {error}
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center py-12">
          <div className="w-6 h-6 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin" />
        </div>
      ) : (
        <>
          {activeTab === 'overview' && <OverviewTab dashboard={dashboard} scans={scans} targets={targets} />}
          {activeTab === 'targets' && <TargetsTab targets={targets} onRefresh={loadData} />}
          {activeTab === 'scans' && <ScansTab scans={scans} onRefresh={loadData} />}
          {activeTab === 'compliance' && <ComplianceTab policies={policies} showForm={showPolicyForm} setShowForm={setShowPolicyForm} form={policyForm} setForm={setPolicyForm} onRefresh={loadData} />}
          {activeTab === 'findings' && <FindingsTab findings={findings} summary={findingSummary} />}
          {activeTab === 'reports' && <ReportsTab scans={scans} />}
          {activeTab === 'schedules' && <SchedulesTab schedules={schedules} targets={targets} onRefresh={loadData} />}
        </>
      )}
      </div>
    </div>
  );
}

// ── Overview Tab ──────────────────────────────────────────────

function OverviewTab({ dashboard, scans, targets }: { dashboard: SecurityDashboard | null; scans: ScanRun[]; targets: ScanTarget[] }) {
  if (!dashboard) return null;

  const recentScans = scans.slice(0, 10);
  const criticalCount = scans.reduce((acc, s) => acc + ((s.result_summary?.critical as number) || 0), 0);
  const highCount = scans.reduce((acc, s) => acc + ((s.result_summary?.high as number) || 0), 0);

  return (
    <div className="space-y-6">
      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <StatCard
          title="Container Vulnerabilities"
          value={criticalCount}
          subtitle={`${highCount} high severity`}
          icon="🛡️"
          color="red"
        />
        <StatCard
          title="Scan Targets"
          value={dashboard.targets.total}
          subtitle={`${dashboard.targets.enabled} enabled`}
          icon="🎯"
          color="blue"
        />
        <StatCard
          title="Recent Scans"
          value={dashboard.scan_summary.completed}
          subtitle={`${dashboard.scan_summary.failed} failed`}
          icon="📋"
          color="green"
        />
        <StatCard
          title="Active Schedules"
          value={dashboard.schedules.enabled}
          subtitle={`${dashboard.schedules.total} total`}
          icon="⏰"
          color="purple"
        />
      </div>

      {/* Recent Scan Activity */}
      <div className="bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)] p-4">
        <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-4">Recent Scan Activity</h3>
        {recentScans.length === 0 ? (
          <p className="text-[var(--text-secondary)] text-sm">No scans yet. Create a scan target to get started.</p>
        ) : (
          <div className="space-y-2">
            {recentScans.map(scan => (
              <div key={scan.id} className="flex items-center justify-between p-3 bg-[var(--bg-primary)] rounded-lg">
                <div className="flex items-center gap-3">
                  <span className={`px-2 py-1 text-xs rounded border ${STATUS_COLORS[scan.status]}`}>
                    {scan.status}
                  </span>
                  <span className="text-sm font-medium text-[var(--text-primary)]">{scan.target_name || scan.target_ref}</span>
                  <span className="text-xs text-[var(--text-secondary)]">{scan.scanner_type}</span>
                </div>
                <div className="text-xs text-[var(--text-secondary)]">
                  {scan.completed_at ? new Date(scan.completed_at).toLocaleString() : scan.created_at ? new Date(scan.created_at).toLocaleString() : 'N/A'}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Targets Overview */}
      <div className="bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)] p-4">
        <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-4">Scan Coverage</h3>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          {targets.slice(0, 6).map(target => (
            <div key={target.id} className="p-3 bg-[var(--bg-primary)] rounded-lg">
              <div className="flex items-center gap-2 mb-2">
                <BrandIcon name={target.scanner_type === 'sonarqube' ? 'sonarqube' : 'trivy'} size={16} />
                <span className="text-sm font-medium text-[var(--text-primary)] truncate">{target.name}</span>
              </div>
              <div className="text-xs text-[var(--text-secondary)]">{target.target_ref}</div>
              {target.last_scan_at && (
                <div className="text-xs text-[var(--text-tertiary)] mt-1">
                  Last scan: {new Date(target.last_scan_at).toLocaleDateString()}
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function StatCard({ title, value, subtitle, icon, color }: { title: string; value: number; subtitle: string; icon: string; color: string }) {
  const colorClasses: Record<string, string> = {
    red: 'from-red-500/10 to-red-500/5 border-red-500/20',
    blue: 'from-blue-500/10 to-blue-500/5 border-blue-500/20',
    green: 'from-emerald-500/10 to-emerald-500/5 border-emerald-500/20',
    purple: 'from-purple-500/10 to-purple-500/5 border-purple-500/20',
  };
  return (
    <div className={`p-4 rounded-lg border bg-gradient-to-br ${colorClasses[color]}`}>
      <div className="flex items-center justify-between mb-2">
        <span className="text-2xl">{icon}</span>
        <span className="text-3xl font-bold text-[var(--text-primary)]">{value}</span>
      </div>
      <div className="text-sm font-medium text-[var(--text-primary)]">{title}</div>
      <div className="text-xs text-[var(--text-secondary)]">{subtitle}</div>
    </div>
  );
}

// ── Targets Tab ───────────────────────────────────────────────

function TargetsTab({ targets, onRefresh }: { targets: ScanTarget[]; onRefresh: () => void }) {
  const [showCreate, setShowCreate] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ScanTarget | null>(null);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    try {
      await securityScan.deleteTarget(deleteTarget.id);
      setDeleteTarget(null);
      onRefresh();
    } catch (e) {
      console.error('Failed to delete target:', e);
    }
  };

  const handleScan = async (id: string) => {
    try {
      await securityScan.triggerScan(id);
      onRefresh();
    } catch (e) {
      console.error('Failed to trigger scan:', e);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">Scan Targets</h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 text-sm"
        >
          + New Target
        </button>
      </div>

      {targets.length === 0 ? (
        <div className="text-center py-12 bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)]">
          <p className="text-[var(--text-secondary)]">No scan targets configured.</p>
          <p className="text-sm text-[var(--text-tertiary)] mt-2">Create a target to start scanning.</p>
        </div>
      ) : (
        <div className="grid gap-4">
          {targets.map(target => (
            <div key={target.id} className="p-4 bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)]">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-lg" style={{ backgroundColor: `${SCANNER_COLORS[target.scanner_type]}20` }}>
                    <BrandIcon name={target.scanner_type === 'sonarqube' ? 'sonarqube' : 'trivy'} size={24} />
                  </div>
                  <div>
                    <h3 className="font-medium text-[var(--text-primary)]">{target.name}</h3>
                    <p className="text-sm text-[var(--text-secondary)]">{target.target_ref}</p>
                    <div className="flex gap-2 mt-1">
                      <span className="text-xs px-2 py-0.5 rounded bg-[var(--bg-primary)] text-[var(--text-tertiary)]">
                        {target.target_type}
                      </span>
                      <span className="text-xs px-2 py-0.5 rounded" style={{ backgroundColor: `${SCANNER_COLORS[target.scanner_type]}20`, color: SCANNER_COLORS[target.scanner_type] }}>
                        {target.scanner_type}
                      </span>
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <span className={`px-2 py-1 text-xs rounded border ${target.enabled ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20' : 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]'}`}>
                    {target.enabled ? 'Enabled' : 'Disabled'}
                  </span>
                </div>
              </div>
              {target.last_scan_at && (
                <div className="mt-3 pt-3 border-t border-[var(--border)] flex items-center justify-between text-sm">
                  <span className="text-[var(--text-secondary)]">
                    Last scan: {new Date(target.last_scan_at).toLocaleString()}
                  </span>
                  <span className={`px-2 py-0.5 text-xs rounded ${target.last_scan_status === 'completed' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>
                    {target.last_scan_status}
                  </span>
                </div>
              )}
              <div className="mt-3 flex gap-2">
                <button
                  onClick={() => handleScan(target.id)}
                  className="px-3 py-1.5 text-xs bg-blue-500 text-white rounded hover:bg-blue-600"
                >
                  Scan Now
                </button>
                <button
                  onClick={() => setDeleteTarget(target)}
                  className="px-3 py-1.5 text-xs bg-red-500/10 text-red-500 rounded hover:bg-red-500/20"
                >
                  Delete
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {showCreate && <CreateTargetModal onClose={() => setShowCreate(false)} onCreated={onRefresh} />}
      {deleteTarget && (
        <ConfirmModal
          open={true}
          title="Delete Scan Target"
          description={`Are you sure you want to delete "${deleteTarget.name}"? This will also delete all associated scan runs and schedules.`}
          onConfirm={handleDelete}
          onCancel={() => setDeleteTarget(null)}
          variant="danger"
        />
      )}
    </div>
  );
}

type ScanCategory = 'code' | 'containers';
type CodeSource = 'local' | 'git_url' | 'connection';
type ContainerSource = 'image' | 'registry' | 'sonarqube';

function CreateTargetModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [category, setCategory] = useState<ScanCategory>('containers');
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');

  // Code source state
  const [codeSource, setCodeSource] = useState<CodeSource>('git_url');
  const [localPath, setLocalPath] = useState('');
  const [gitUrl, setGitUrl] = useState('');
  const [gitConnections, setGitConnections] = useState<Connection[]>([]);
  const [selectedConnectionId, setSelectedConnectionId] = useState('');
  const [connectionRepoUrl, setConnectionRepoUrl] = useState('');

  // Container source state
  const [containerSource, setContainerSource] = useState<ContainerSource>('image');
  const [imageRef, setImageRef] = useState('');
  const [sonarProjectKey, setSonarProjectKey] = useState('');

  // Registry cascading selector state
  const [registryRepos, setRegistryRepos] = useState<RegistryRepository[]>([]);
  const [selectedRepoId, setSelectedRepoId] = useState('');
  const [registryScope, setRegistryScope] = useState<'all' | 'specific'>('specific');
  const [registryImages, setRegistryImages] = useState<string[]>([]);
  const [loadingImages, setLoadingImages] = useState(false);
  const [selectedImage, setSelectedImage] = useState('');
  const [imageSearch, setImageSearch] = useState('');
  const [registryTags, setRegistryTags] = useState<string[]>([]);
  const [loadingTags, setLoadingTags] = useState(false);
  const [selectedTag, setSelectedTag] = useState('');
  const [registryManualRef, setRegistryManualRef] = useState('');

  const registryHost = (url: string) => url.replace(/^https?:\/\//, '').replace(/\/$/, '');

  // Load data on mount
  useEffect(() => {
    registryRepositories.list().then(res => setRegistryRepos(res.registry_repositories || [])).catch(() => {});
    connections.list().then(res => {
      const allConns = (res as { connections: Connection[] }).connections || [];
      setGitConnections(allConns.filter(c => c.type === 'gitlab' || c.type === 'git'));
    }).catch(() => {});
  }, []);

  // Registry cascading handlers
  const handleRepoSelect = async (repoId: string) => {
    setSelectedRepoId(repoId);
    setSelectedImage('');
    setSelectedTag('');
    setRegistryImages([]);
    setRegistryTags([]);
    setImageSearch('');
    setRegistryManualRef('');
    const repo = registryRepos.find(r => r.id === repoId);
    const host = repo ? registryHost(repo.url) : '';
    if (registryScope === 'all' && host) setRegistryManualRef(host);
    if (!repoId || registryScope === 'all') return;
    setLoadingImages(true);
    try {
      const resImg = await registryRepositories.listImages(repoId);
      setRegistryImages(resImg.images || []);
    } catch {
      setRegistryImages([]);
    } finally {
      setLoadingImages(false);
    }
  };

  const handleScopeChange = (scope: 'all' | 'specific') => {
    setRegistryScope(scope);
    setSelectedImage('');
    setSelectedTag('');
    setRegistryTags([]);
    setImageSearch('');
    if (scope === 'all' && selectedRepoId) {
      const repo = registryRepos.find(r => r.id === selectedRepoId);
      setRegistryManualRef(repo ? registryHost(repo.url) : '');
    } else {
      setRegistryManualRef('');
    }
  };

  const handleImageSelect = async (imageName: string) => {
    setSelectedImage(imageName);
    setSelectedTag('');
    setRegistryTags([]);
    setRegistryManualRef('');
    if (!imageName || !selectedRepoId) return;
    setLoadingTags(true);
    try {
      const res = await registryRepositories.listTags(selectedRepoId, imageName);
      setRegistryTags(res.tags || []);
    } catch {
      setRegistryTags([]);
    } finally {
      setLoadingTags(false);
    }
  };

  const handleTagSelect = (tag: string) => {
    setSelectedTag(tag);
    if (tag && selectedImage && selectedRepoId) {
      const repo = registryRepos.find(r => r.id === selectedRepoId);
      const host = repo ? registryHost(repo.url) : '';
      setRegistryManualRef(`${host}/${selectedImage}:${tag}`);
    }
  };

  // Resolve final target_type and target_ref from category/source state
  const resolveTarget = (): { target_type: TargetType; target_ref: string; connection_id?: string } | null => {
    if (category === 'code') {
      switch (codeSource) {
        case 'local':
          return localPath ? { target_type: 'filesystem', target_ref: localPath } : null;
        case 'git_url':
          return gitUrl ? { target_type: 'git_repo', target_ref: gitUrl } : null;
        case 'connection':
          return (selectedConnectionId && connectionRepoUrl)
            ? { target_type: 'git_repo', target_ref: connectionRepoUrl, connection_id: selectedConnectionId }
            : null;
      }
    } else {
      switch (containerSource) {
        case 'image':
          return imageRef ? { target_type: 'image', target_ref: imageRef } : null;
        case 'registry':
          return registryManualRef ? { target_type: 'registry', target_ref: registryManualRef } : null;
        case 'sonarqube':
          return sonarProjectKey ? { target_type: 'sonarqube_project', target_ref: sonarProjectKey } : null;
      }
    }
  };

  const resolveScanner = (): ScannerType => {
    if (category === 'code') return 'sonarqube';
    return containerSource === 'sonarqube' ? 'sonarqube' : 'trivy';
  };

  const handleSubmit = async () => {
    if (!name.trim()) { setError('Name is required'); return; }
    const resolved = resolveTarget();
    if (!resolved) { setError('Target reference is required'); return; }
    setError('');
    setCreating(true);
    try {
      await securityScan.createTarget({
        name: name.trim(),
        scanner_type: resolveScanner(),
        target_type: resolved.target_type,
        target_ref: resolved.target_ref,
        connection_id: resolved.connection_id,
        enabled: true,
        scan_config: {},
      });
      onCreated();
      onClose();
    } catch (e) {
      setError(friendlyError(e).message);
    } finally {
      setCreating(false);
    }
  };

  const resolved = resolveTarget();
  const canSubmit = name.trim() && resolved;

  const connectionIcon = (type: string) => {
    if (type === 'gitlab') return '\u{1F98A}';
    return '\u{1F517}';
  };

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50">
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-xl mx-4 max-h-[90vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Create Scan Target</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
          {error && (
            <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3 text-[12px] text-red-500">{error}</div>
          )}

          {/* Name */}
          <div>
            <label className="label">Name *</label>
            <input type="text" value={name} onChange={e => setName(e.target.value)} className="input" placeholder="My App Scan" />
          </div>

          {/* Category Tabs */}
          <div>
            <label className="label">Scan Category</label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setCategory('code')}
                className={`p-3 rounded-lg border text-left transition-all ${
                  category === 'code'
                    ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                    : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                }`}
              >
                <div className="text-[13px] font-medium">Code Scanning</div>
                <div className="text-[10px] text-[var(--text-tertiary)] mt-0.5">SonarQube code quality &amp; security analysis</div>
              </button>
              <button
                type="button"
                onClick={() => setCategory('containers')}
                className={`p-3 rounded-lg border text-left transition-all ${
                  category === 'containers'
                    ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                    : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                }`}
              >
                <div className="text-[13px] font-medium">Container Scanning</div>
                <div className="text-[10px] text-[var(--text-tertiary)] mt-0.5">Trivy vulnerability scanning for images</div>
              </button>
            </div>
          </div>

          {/* ── CODE category ── */}
          {category === 'code' && (
            <div className="space-y-4">
              <div>
                <label className="label">Source</label>
                <div className="grid grid-cols-3 gap-2">
                  <button
                    type="button"
                    onClick={() => setCodeSource('local')}
                    className={`p-2.5 rounded-lg border text-center transition-all text-[11px] ${
                      codeSource === 'local'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    Local Folder
                  </button>
                  <button
                    type="button"
                    onClick={() => setCodeSource('git_url')}
                    className={`p-2.5 rounded-lg border text-center transition-all text-[11px] ${
                      codeSource === 'git_url'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    Git URL
                  </button>
                  <button
                    type="button"
                    onClick={() => setCodeSource('connection')}
                    className={`p-2.5 rounded-lg border text-center transition-all text-[11px] ${
                      codeSource === 'connection'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    Connection
                  </button>
                </div>
              </div>

              {codeSource === 'local' && (
                <div>
                  <label className="label">Local Path *</label>
                  <input type="text" value={localPath} onChange={e => setLocalPath(e.target.value)} className="input font-mono text-[12px]" placeholder="/path/to/project" />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Absolute path on the server filesystem</p>
                </div>
              )}

              {codeSource === 'git_url' && (
                <div>
                  <label className="label">Git Repository URL *</label>
                  <input type="text" value={gitUrl} onChange={e => setGitUrl(e.target.value)} className="input font-mono text-[12px]" placeholder="https://github.com/org/repo.git" />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Any Git repository URL (HTTPS or SSH)</p>
                </div>
              )}

              {codeSource === 'connection' && (
                <div className="space-y-3">
                  <div>
                    <label className="label">Connection *</label>
                    {gitConnections.length === 0 ? (
                      <div className="p-3 rounded-lg border border-amber-500/20 bg-amber-500/10">
                        <p className="text-[11px] text-amber-600 dark:text-amber-400">
                          No Git connections found. <a href="/connections" className="underline font-medium">Add a connection</a> (GitLab or Git) first.
                        </p>
                      </div>
                    ) : (
                      <select
                        value={selectedConnectionId}
                        onChange={e => { setSelectedConnectionId(e.target.value); setConnectionRepoUrl(''); }}
                        className="input text-[12px]"
                        style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}
                      >
                        <option value="" style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>Select connection...</option>
                        {gitConnections.map(c => (
                          <option key={c.id} value={c.id} style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>
                            {connectionIcon(c.type)} {c.name} ({c.type})
                          </option>
                        ))}
                      </select>
                    )}
                  </div>
                  {selectedConnectionId && (
                    <div>
                      <label className="label">Repository URL *</label>
                      <input type="text" value={connectionRepoUrl} onChange={e => setConnectionRepoUrl(e.target.value)} className="input font-mono text-[12px]" placeholder="https://gitlab.com/org/repo or group/project" />
                      <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Repository path or URL from the selected connection</p>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {/* ── CONTAINERS category ── */}
          {category === 'containers' && (
            <div className="space-y-4">
              <div>
                <label className="label">Source</label>
                <div className="grid grid-cols-3 gap-2">
                  <button
                    type="button"
                    onClick={() => setContainerSource('image')}
                    className={`p-2.5 rounded-lg border text-center transition-all text-[11px] ${
                      containerSource === 'image'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    Image Ref
                  </button>
                  <button
                    type="button"
                    onClick={() => setContainerSource('registry')}
                    className={`p-2.5 rounded-lg border text-center transition-all text-[11px] ${
                      containerSource === 'registry'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    Registry Repo
                  </button>
                  <button
                    type="button"
                    onClick={() => setContainerSource('sonarqube')}
                    className={`p-2.5 rounded-lg border text-center transition-all text-[11px] ${
                      containerSource === 'sonarqube'
                        ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium'
                        : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                    }`}
                  >
                    SonarQube
                  </button>
                </div>
              </div>

              {containerSource === 'image' && (
                <div>
                  <label className="label">Image Reference *</label>
                  <input type="text" value={imageRef} onChange={e => setImageRef(e.target.value)} className="input font-mono text-[12px]" placeholder="nginx:latest" />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">e.g. nginx:latest, ghcr.io/org/image:tag</p>
                </div>
              )}

              {containerSource === 'registry' && (
                <div className="space-y-2">
                  {registryRepos.length === 0 ? (
                    <div className="p-3 rounded-lg border border-amber-500/20 bg-amber-500/10">
                      <p className="text-[11px] text-amber-600 dark:text-amber-400">
                        No registry repositories configured. <a href="/registry-repositories" className="underline font-medium">Add a registry</a> first.
                      </p>
                    </div>
                  ) : (
                    <>
                      <div>
                        <label className="label text-[11px]">Registry *</label>
                        <select
                          value={selectedRepoId}
                          onChange={e => handleRepoSelect(e.target.value)}
                          className="input text-[12px]"
                          style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}
                        >
                          <option value="" style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>Select registry...</option>
                          {registryRepos.map(repo => (
                            <option key={repo.id} value={repo.id} style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>
                              {repo.name} ({repo.url.replace(/^https?:\/\//, '')})
                            </option>
                          ))}
                        </select>
                      </div>
                      {selectedRepoId && (
                        <div className="flex gap-2">
                          <button type="button" onClick={() => handleScopeChange('all')} className={`flex-1 text-[11px] px-3 py-1.5 rounded-lg border transition-all ${registryScope === 'all' ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium' : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'}`}>
                            All images (whole registry)
                          </button>
                          <button type="button" onClick={() => handleScopeChange('specific')} className={`flex-1 text-[11px] px-3 py-1.5 rounded-lg border transition-all ${registryScope === 'specific' ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)] font-medium' : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'}`}>
                            Specific image
                          </button>
                        </div>
                      )}
                      {registryScope === 'specific' && selectedRepoId && (
                        <>
                          {registryImages.length > 5 && (
                            <input type="text" value={imageSearch} onChange={e => setImageSearch(e.target.value)} className="input text-[12px]" placeholder="Filter images..." />
                          )}
                          <select value={selectedImage} onChange={e => handleImageSelect(e.target.value)} className="input font-mono text-[12px]" disabled={loadingImages} style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>
                            <option value="" style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>{loadingImages ? 'Loading images...' : 'Select image...'}</option>
                            {registryImages.filter(img => !imageSearch || img.toLowerCase().includes(imageSearch.toLowerCase())).map(img => (
                              <option key={img} value={img} style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>{img}</option>
                            ))}
                          </select>
                          {selectedImage && (
                            <select value={selectedTag} onChange={e => handleTagSelect(e.target.value)} className="input font-mono text-[12px]" disabled={loadingTags} style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>
                              <option value="" style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>{loadingTags ? 'Loading tags...' : 'Select tag...'}</option>
                              {registryTags.map(tag => (
                                <option key={tag} value={tag} style={{ backgroundColor: 'var(--surface)', color: 'var(--text-primary)' }}>{tag}</option>
                              ))}
                            </select>
                          )}
                        </>
                      )}
                      {registryManualRef && (
                        <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-[var(--bg-primary)] border border-[var(--border)]">
                          <span className="text-[11px] text-[var(--text-secondary)]">{registryScope === 'all' ? 'Registry:' : 'Ref:'}</span>
                          <span className="text-[12px] font-mono text-[var(--text-primary)]">{registryManualRef}</span>
                        </div>
                      )}
                      <input type="text" value={registryManualRef} onChange={e => setRegistryManualRef(e.target.value)} className="input font-mono text-[12px]" placeholder="Or type manually: ghcr.io/org/image:tag" />
                    </>
                  )}
                </div>
              )}

              {containerSource === 'sonarqube' && (
                <div>
                  <label className="label">SonarQube Project Key *</label>
                  <input type="text" value={sonarProjectKey} onChange={e => setSonarProjectKey(e.target.value)} className="input font-mono text-[12px]" placeholder="my-project-key" />
                  <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Project key from your SonarQube instance</p>
                </div>
              )}
            </div>
          )}

          {/* Summary */}
          {resolved && (
            <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-[var(--bg-secondary)] border border-[var(--border)]">
              <span className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider">Target:</span>
              <span className="text-[12px] font-mono text-[var(--text-primary)]">{resolved.target_ref}</span>
              <span className="text-[10px] px-1.5 py-0.5 rounded bg-[var(--bg-primary)] text-[var(--text-tertiary)] ml-auto">{resolved.target_type}</span>
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button
            onClick={handleSubmit}
            disabled={creating || !canSubmit}
            className="btn btn-primary"
          >
            {creating ? 'Creating...' : 'Create Target'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Scans Tab ─────────────────────────────────────────────────

function ScansTab({ scans, onRefresh }: { scans: ScanRun[]; onRefresh: () => void }) {
  const [expandedScan, setExpandedScan] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<string>('');

  const filteredScans = statusFilter ? scans.filter(s => s.status === statusFilter) : scans;

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">Scan Results</h2>
        <div className="flex gap-2">
          <select
            value={statusFilter}
            onChange={e => setStatusFilter(e.target.value)}
            className="px-3 py-1.5 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-sm text-[var(--text-primary)]"
          >
            <option value="">All Status</option>
            <option value="completed">Completed</option>
            <option value="failed">Failed</option>
            <option value="running">Running</option>
            <option value="pending">Pending</option>
          </select>
          <button onClick={onRefresh} className="px-3 py-1.5 text-sm bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg hover:bg-[var(--bg-primary)]">
            Refresh
          </button>
        </div>
      </div>

      {filteredScans.length === 0 ? (
        <div className="text-center py-12 bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)]">
          <p className="text-[var(--text-secondary)]">No scan results found.</p>
        </div>
      ) : (
        <div className="bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)] overflow-hidden">
          <table className="w-full">
            <thead className="bg-[var(--bg-primary)]">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Target</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Scanner</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Status</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Severity</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Duration</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Triggered</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {filteredScans.map(scan => (
                <Fragment key={scan.id}>
                  <tr
                    onClick={() => setExpandedScan(expandedScan === scan.id ? null : scan.id)}
                    className="cursor-pointer hover:bg-[var(--bg-primary)]"
                  >
                    <td className="px-4 py-3 text-sm text-[var(--text-primary)]">{scan.target_name || scan.target_ref || 'Unknown'}</td>
                    <td className="px-4 py-3">
                      <span className="text-xs px-2 py-0.5 rounded" style={{ backgroundColor: `${SCANNER_COLORS[scan.scanner_type]}20`, color: SCANNER_COLORS[scan.scanner_type] }}>
                        {scan.scanner_type}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 text-xs rounded border ${STATUS_COLORS[scan.status]}`}>{scan.status}</span>
                    </td>
                    <td className="px-4 py-3">
                      {scan.result_summary && (
                        <div className="flex gap-1">
                          {(scan.result_summary.critical as number) > 0 && (
                            <span className="text-xs px-1.5 py-0.5 rounded bg-red-500/10 text-red-500">{scan.result_summary.critical as number}C</span>
                          )}
                          {(scan.result_summary.high as number) > 0 && (
                            <span className="text-xs px-1.5 py-0.5 rounded bg-orange-500/10 text-orange-500">{scan.result_summary.high as number}H</span>
                          )}
                          {(scan.result_summary.medium as number) > 0 && (
                            <span className="text-xs px-1.5 py-0.5 rounded bg-yellow-500/10 text-yellow-600">{scan.result_summary.medium as number}M</span>
                          )}
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3 text-sm text-[var(--text-secondary)]">
                      {scan.duration_ms ? `${(scan.duration_ms / 1000).toFixed(1)}s` : '-'}
                    </td>
                    <td className="px-4 py-3 text-sm text-[var(--text-secondary)]">{scan.trigger_type}</td>
                  </tr>
                  {expandedScan === scan.id && scan.result_full && (
                    <tr>
                      <td colSpan={6} className="px-4 py-3 bg-[var(--bg-primary)]">
                        <pre className="text-xs text-[var(--text-secondary)] overflow-x-auto max-h-60">
                          {JSON.stringify(scan.result_full, null, 2)}
                        </pre>
                      </td>
                    </tr>
                  )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ── Reports Tab ───────────────────────────────────────────────

function ReportsTab({ scans }: { scans: ScanRun[] }) {
  const completedScans = scans.filter(s => s.status === 'completed' && s.result_summary);

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-[var(--text-primary)]">Reports</h2>

      {completedScans.length === 0 ? (
        <div className="text-center py-12 bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)]">
          <p className="text-[var(--text-secondary)]">No completed scans to report.</p>
        </div>
      ) : (
        <div className="grid gap-4">
          {completedScans.map(scan => (
            <div key={scan.id} className="p-4 bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)]">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <h3 className="font-medium text-[var(--text-primary)]">{scan.target_name || scan.target_ref}</h3>
                  <p className="text-sm text-[var(--text-secondary)]">
                    {new Date(scan.completed_at || scan.created_at).toLocaleString()}
                  </p>
                </div>
                <span className="text-xs px-2 py-1 rounded" style={{ backgroundColor: `${SCANNER_COLORS[scan.scanner_type]}20`, color: SCANNER_COLORS[scan.scanner_type] }}>
                  {scan.scanner_type}
                </span>
              </div>

              {scan.result_summary && (
                <div className="grid grid-cols-5 gap-2">
                  {Object.entries(scan.result_summary).filter(([k]) => ['critical', 'high', 'medium', 'low', 'unknown'].includes(k)).map(([key, value]) => (
                    <div key={key} className="text-center p-2 rounded bg-[var(--bg-primary)]">
                      <div className="text-lg font-bold" style={{ color: SEVERITY_COLORS[key] }}>{value as number}</div>
                      <div className="text-xs text-[var(--text-secondary)] capitalize">{key}</div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Schedules Tab ─────────────────────────────────────────────

function SchedulesTab({ schedules, targets, onRefresh }: { schedules: ScanSchedule[]; targets: ScanTarget[]; onRefresh: () => void }) {
  const [showCreate, setShowCreate] = useState(false);
  const [deleteSchedule, setDeleteSchedule] = useState<ScanSchedule | null>(null);

  const handleDelete = async () => {
    if (!deleteSchedule) return;
    try {
      await securityScan.deleteSchedule(deleteSchedule.id);
      setDeleteSchedule(null);
      onRefresh();
    } catch (e) {
      console.error('Failed to delete schedule:', e);
    }
  };

  const handleToggle = async (schedule: ScanSchedule) => {
    try {
      await securityScan.updateSchedule(schedule.id, { enabled: !schedule.enabled });
      onRefresh();
    } catch (e) {
      console.error('Failed to toggle schedule:', e);
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h2 className="text-lg font-semibold text-[var(--text-primary)]">Scan Schedules</h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 text-sm"
        >
          + New Schedule
        </button>
      </div>

      {schedules.length === 0 ? (
        <div className="text-center py-12 bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)]">
          <p className="text-[var(--text-secondary)]">No schedules configured.</p>
        </div>
      ) : (
        <div className="bg-[var(--bg-secondary)] rounded-lg border border-[var(--border)] overflow-hidden">
          <table className="w-full">
            <thead className="bg-[var(--bg-primary)]">
              <tr>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Target</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Cron</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Next Run</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Last Run</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Status</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-[var(--text-secondary)] uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {schedules.map(schedule => (
                <tr key={schedule.id}>
                  <td className="px-4 py-3 text-sm text-[var(--text-primary)]">{schedule.target_name || 'Unknown'}</td>
                  <td className="px-4 py-3 text-sm font-mono text-[var(--text-secondary)]">{schedule.cron_expression}</td>
                  <td className="px-4 py-3 text-sm text-[var(--text-secondary)]">
                    {schedule.next_run_at ? new Date(schedule.next_run_at).toLocaleString() : '-'}
                  </td>
                  <td className="px-4 py-3 text-sm text-[var(--text-secondary)]">
                    {schedule.last_run_at ? new Date(schedule.last_run_at).toLocaleString() : 'Never'}
                  </td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => handleToggle(schedule)}
                      className={`px-2 py-1 text-xs rounded border ${schedule.enabled ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20' : 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]'}`}
                    >
                      {schedule.enabled ? 'Enabled' : 'Disabled'}
                    </button>
                  </td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => setDeleteSchedule(schedule)}
                      className="text-xs text-red-500 hover:text-red-600"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showCreate && <CreateScheduleModal targets={targets} onClose={() => setShowCreate(false)} onCreated={onRefresh} />}
      {deleteSchedule && (
        <ConfirmModal
          open={true}
          title="Delete Schedule"
          description="Are you sure you want to delete this schedule?"
          onConfirm={handleDelete}
          onCancel={() => setDeleteSchedule(null)}
          variant="danger"
        />
      )}
    </div>
  );
}

function CreateScheduleModal({ targets, onClose, onCreated }: { targets: ScanTarget[]; onClose: () => void; onCreated: () => void }) {
  const [targetId, setTargetId] = useState('');
  const [cronExpression, setCronExpression] = useState('0 2 * * *');
  const [creating, setCreating] = useState(false);

  const presets = [
    { label: 'Daily at 2am', value: '0 2 * * *' },
    { label: 'Every 6 hours', value: '0 */6 * * *' },
    { label: 'Weekly on Monday', value: '0 2 * * 1' },
    { label: 'Every hour', value: '0 * * * *' },
  ];

  const handleSubmit = async () => {
    if (!targetId || !cronExpression) return;
    setCreating(true);
    try {
      await securityScan.createSchedule({ target_id: targetId, cron_expression: cronExpression, enabled: true });
      onCreated();
      onClose();
    } catch (e) {
      console.error('Failed to create schedule:', e);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50">
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] flex flex-col">
        <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Create Scan Schedule</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
          <div>
            <label className="label">Target</label>
            <select
              value={targetId}
              onChange={e => setTargetId(e.target.value)}
              className="input"
            >
              <option value="">Select a target...</option>
              {targets.map(t => (
                <option key={t.id} value={t.id}>{t.name} ({t.target_ref})</option>
              ))}
            </select>
          </div>

          <div>
            <label className="label">Schedule Preset</label>
            <div className="grid grid-cols-2 gap-2">
              {presets.map(p => (
                <button
                  key={p.value}
                  onClick={() => setCronExpression(p.value)}
                  className={`px-3 py-2 text-sm rounded-lg border transition-all ${
                    cronExpression === p.value
                      ? 'border-[var(--accent)] bg-[var(--accent-subtle)] text-[var(--accent)]'
                      : 'border-[var(--border)] text-[var(--text-secondary)] hover:border-[var(--text-tertiary)]'
                  }`}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="label">Cron Expression</label>
            <input
              type="text"
              value={cronExpression}
              onChange={e => setCronExpression(e.target.value)}
              className="input font-mono text-[12px]"
              placeholder="0 2 * * *"
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
          <button onClick={onClose} className="btn btn-secondary">Cancel</button>
          <button
            onClick={handleSubmit}
            disabled={creating || !targetId || !cronExpression}
            className="btn btn-primary"
          >
            {creating ? 'Creating...' : 'Create Schedule'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Compliance Tab ──────────────────────────────────────────────

const COMPLAINT_SEVERITY_COLORS: Record<string, string> = {
  critical: 'bg-red-500/10 text-red-500 border-red-500/20',
  high: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
  medium: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  low: 'bg-green-500/10 text-green-500 border-green-500/20',
};

function ComplianceTab({ policies, showForm, setShowForm, form, setForm, onRefresh }: {
  policies: CompliancePolicy[];
  showForm: boolean;
  setShowForm: (v: boolean) => void;
  form: { name: string; description: string; policy_type: CompliancePolicy['policy_type']; environment: string; severity: CompliancePolicy['severity']; blocking: boolean; enabled: boolean };
  setForm: (v: typeof form) => void;
  onRefresh: () => void;
}) {
  const [deleting, setDeleting] = useState<string | null>(null);

  const handleCreate = async () => {
    try {
      await devops.createPolicy(form);
      setShowForm(false);
      setForm({ name: '', description: '', policy_type: 'resource_limits', environment: 'production', severity: 'high', blocking: true, enabled: true });
      onRefresh();
    } catch { /* ignore */ }
  };

  const handleDelete = async (id: string) => {
    try {
      await devops.deletePolicy(id);
      setDeleting(null);
      onRefresh();
    } catch { /* ignore */ }
  };

  const handleToggle = async (p: CompliancePolicy) => {
    try {
      await devops.updatePolicy(p.id, { enabled: !p.enabled });
      onRefresh();
    } catch { /* ignore */ }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-base font-semibold text-[var(--text-primary)]">Compliance Policies</h3>
          <p className="text-xs text-[var(--text-secondary)]">Define rules that must pass before deployments are allowed</p>
        </div>
        <button onClick={() => setShowForm(!showForm)} className="px-3 py-1.5 text-sm bg-blue-500 text-white rounded-lg hover:bg-blue-600">
          {showForm ? 'Cancel' : '+ Add Policy'}
        </button>
      </div>

      {showForm && (
        <div className="bg-[var(--bg-secondary)] rounded-lg p-4 border border-[var(--border)]">
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="text-xs text-[var(--text-secondary)]">Name</label>
              <input type="text" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="w-full mt-1 px-3 py-1.5 text-sm bg-[var(--bg-primary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]" placeholder="Resource limits required" />
            </div>
            <div>
              <label className="text-xs text-[var(--text-secondary)]">Type</label>
              <select value={form.policy_type} onChange={e => setForm({ ...form, policy_type: e.target.value as CompliancePolicy['policy_type'] })} className="w-full mt-1 px-3 py-1.5 text-sm bg-[var(--bg-primary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]">
                <option value="resource_limits">Resource Limits</option>
                <option value="security_scan">Security Scan</option>
                <option value="required_labels">Required Labels</option>
                <option value="custom">Custom</option>
              </select>
            </div>
            <div>
              <label className="text-xs text-[var(--text-secondary)]">Environment</label>
              <input type="text" value={form.environment} onChange={e => setForm({ ...form, environment: e.target.value })} className="w-full mt-1 px-3 py-1.5 text-sm bg-[var(--bg-primary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]" />
            </div>
            <div>
              <label className="text-xs text-[var(--text-secondary)]">Severity</label>
              <select value={form.severity} onChange={e => setForm({ ...form, severity: e.target.value as CompliancePolicy['severity'] })} className="w-full mt-1 px-3 py-1.5 text-sm bg-[var(--bg-primary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]">
                <option value="critical">Critical</option>
                <option value="high">High</option>
                <option value="medium">Medium</option>
                <option value="low">Low</option>
              </select>
            </div>
            <div className="col-span-2">
              <label className="text-xs text-[var(--text-secondary)]">Description</label>
              <input type="text" value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="w-full mt-1 px-3 py-1.5 text-sm bg-[var(--bg-primary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]" />
            </div>
          </div>
          <div className="flex items-center gap-4 mt-3">
            <label className="flex items-center gap-2"><input type="checkbox" checked={form.blocking} onChange={e => setForm({ ...form, blocking: e.target.checked })} className="rounded" /><span className="text-sm text-[var(--text-secondary)]">Blocking</span></label>
            <label className="flex items-center gap-2"><input type="checkbox" checked={form.enabled} onChange={e => setForm({ ...form, enabled: e.target.checked })} className="rounded" /><span className="text-sm text-[var(--text-secondary)]">Enabled</span></label>
          </div>
          <button onClick={handleCreate} className="mt-3 px-3 py-1.5 text-sm bg-blue-500 text-white rounded-lg hover:bg-blue-600">Create Policy</button>
        </div>
      )}

      <div className="space-y-2">
        {policies.length === 0 ? (
          <div className="text-center py-8 text-[var(--text-secondary)] text-sm">No compliance policies configured. All deployments pass by default.</div>
        ) : (
          policies.map(p => (
            <div key={p.id} className="bg-[var(--bg-secondary)] rounded-lg p-4 border border-[var(--border)] flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className={`px-2 py-0.5 text-xs rounded-full border ${COMPLAINT_SEVERITY_COLORS[p.severity] || ''}`}>{p.severity}</span>
                <div>
                  <div className="text-sm font-medium text-[var(--text-primary)]">{p.name}</div>
                  <div className="text-xs text-[var(--text-secondary)]">
                    {p.policy_type} · {p.environment}{p.blocking ? ' · Blocking' : ' · Warning'}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button onClick={() => handleToggle(p)} className={`px-2 py-1 text-xs rounded ${p.enabled ? 'bg-emerald-500/10 text-emerald-600' : 'bg-[var(--border-light)] text-[var(--text-tertiary)]'}`}>
                  {p.enabled ? 'Enabled' : 'Disabled'}
                </button>
                <button onClick={() => setDeleting(p.id)} className="px-2 py-1 text-xs text-red-500 hover:text-red-400">Delete</button>
              </div>
              {deleting === p.id && (
                <ConfirmModal open={true} onCancel={() => setDeleting(null)} onConfirm={() => handleDelete(p.id)} title="Delete Policy" description={`Delete "${p.name}"?`} variant="danger" />
              )}
            </div>
          ))
        )}
      </div>
    </div>
  );
}

// ── Findings Tab ──────────────────────────────────────────────

const FINDING_SEVERITY_COLORS: Record<string, string> = {
  critical: 'bg-red-500/10 text-red-500 border-red-500/20',
  high: 'bg-orange-500/10 text-orange-500 border-orange-500/20',
  medium: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  low: 'bg-green-500/10 text-green-500 border-green-500/20',
  info: 'bg-blue-500/10 text-blue-500 border-blue-500/20',
};

const FINDING_STATUS_COLORS: Record<string, string> = {
  open: 'bg-red-500/10 text-red-500',
  acknowledged: 'bg-yellow-500/10 text-yellow-500',
  resolved: 'bg-emerald-500/10 text-emerald-600',
  false_positive: 'bg-[var(--border-light)] text-[var(--text-tertiary)]',
};

function FindingsTab({ findings, summary }: { findings: SecurityFinding[]; summary: SecurityFindingSummary | null }) {
  const [statusFilter, setStatusFilter] = useState('');
  const filtered = statusFilter ? findings.filter(f => f.status === statusFilter) : findings;

  return (
    <div className="space-y-4">
      {summary && (
        <div className="grid grid-cols-5 gap-3">
          {[
            { label: 'Critical', value: summary.by_severity.critical, color: 'text-red-500' },
            { label: 'High', value: summary.by_severity.high, color: 'text-orange-500' },
            { label: 'Medium', value: summary.by_severity.medium, color: 'text-yellow-500' },
            { label: 'Low', value: summary.by_severity.low, color: 'text-green-500' },
            { label: 'Total', value: summary.total, color: 'text-[var(--text-primary)]' },
          ].map(s => (
            <div key={s.label} className="bg-[var(--bg-secondary)] rounded-lg p-3 border border-[var(--border)] text-center">
              <div className={`text-2xl font-bold ${s.color}`}>{s.value}</div>
              <div className="text-xs text-[var(--text-secondary)]">{s.label}</div>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-center gap-3">
        <select value={statusFilter} onChange={e => setStatusFilter(e.target.value)} className="text-xs border border-[var(--border)] rounded-lg px-3 py-1.5 bg-[var(--surface)] text-[var(--text-primary)]">
          <option value="">All statuses</option>
          <option value="open">Open</option>
          <option value="acknowledged">Acknowledged</option>
          <option value="resolved">Resolved</option>
          <option value="false_positive">False Positive</option>
        </select>
        <span className="text-xs text-[var(--text-tertiary)]">{filtered.length} findings</span>
      </div>

      <div className="space-y-2">
        {filtered.length === 0 ? (
          <div className="text-center py-8 text-[var(--text-secondary)] text-sm">No security findings. All clear!</div>
        ) : (
          filtered.slice(0, 50).map(f => (
            <div key={f.id} className="bg-[var(--bg-secondary)] rounded-lg p-4 border border-[var(--border)] flex items-center justify-between">
              <div className="flex items-center gap-3">
                <span className={`px-2 py-0.5 text-xs rounded-full border ${FINDING_SEVERITY_COLORS[f.severity] || ''}`}>{f.severity}</span>
                <div>
                  <div className="text-sm font-medium text-[var(--text-primary)]">{f.title}</div>
                  <div className="text-xs text-[var(--text-secondary)]">{f.finding_type} · {f.resource_type} · {f.environment}</div>
                </div>
              </div>
              <span className={`px-2 py-0.5 text-xs rounded-full ${FINDING_STATUS_COLORS[f.status] || ''}`}>{f.status}</span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
