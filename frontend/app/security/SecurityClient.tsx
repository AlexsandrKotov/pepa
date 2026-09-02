'use client';

import { useState, useEffect, useCallback, Fragment } from 'react';
import { securityScan, type ScanTarget, type ScanRun, type ScanSchedule, type SecurityDashboard, type ScannerType, type TargetType } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import BrandIcon from '@/components/BrandIcon';
import ConfirmModal from '@/components/ConfirmModal';

type TabKey = 'overview' | 'targets' | 'scans' | 'reports' | 'schedules';

const TABS: { key: TabKey; label: string; icon: string }[] = [
  { key: 'overview', label: 'Overview', icon: '📊' },
  { key: 'targets', label: 'Scan Targets', icon: '🎯' },
  { key: 'scans', label: 'Scan Results', icon: '📋' },
  { key: 'reports', label: 'Reports', icon: '📄' },
  { key: 'schedules', label: 'Schedules', icon: '⏰' },
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

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [dash, tgts, scns, scheds] = await Promise.all([
        securityScan.getDashboard(),
        securityScan.listTargets(),
        securityScan.listScans({ limit: 50 }),
        securityScan.listSchedules(),
      ]);
      setDashboard(dash);
      setTargets(tgts || []);
      setScans(scns || []);
      setSchedules(scheds || []);
    } catch (e) {
      setError(friendlyError(e).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-[var(--text-primary)]">Security Scanning</h1>
          <p className="text-sm text-[var(--text-secondary)]">Manage vulnerability scans with Trivy and SonarQube</p>
        </div>
        <button
          onClick={() => securityScan.scanAll()}
          className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 transition-colors"
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
            <span className="mr-2">{tab.icon}</span>
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
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
        </div>
      ) : (
        <>
          {activeTab === 'overview' && <OverviewTab dashboard={dashboard} scans={scans} targets={targets} />}
          {activeTab === 'targets' && <TargetsTab targets={targets} onRefresh={loadData} />}
          {activeTab === 'scans' && <ScansTab scans={scans} onRefresh={loadData} />}
          {activeTab === 'reports' && <ReportsTab scans={scans} />}
          {activeTab === 'schedules' && <SchedulesTab schedules={schedules} targets={targets} onRefresh={loadData} />}
        </>
      )}
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

function CreateTargetModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [scannerType, setScannerType] = useState<ScannerType>('trivy');
  const [targetType, setTargetType] = useState<TargetType>('image');
  const [targetRef, setTargetRef] = useState('');
  const [creating, setCreating] = useState(false);

  const handleSubmit = async () => {
    if (!name || !targetRef) return;
    setCreating(true);
    try {
      await securityScan.createTarget({
        name,
        scanner_type: scannerType,
        target_type: targetType,
        target_ref: targetRef,
        enabled: true,
        scan_config: {},
      });
      onCreated();
      onClose();
    } catch (e) {
      console.error('Failed to create target:', e);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-[var(--bg-primary)] rounded-lg border border-[var(--border)] p-6 w-full max-w-md">
        <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-4">Create Scan Target</h2>
        
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Name</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]"
              placeholder="My App Scan"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Scanner</label>
            <select
              value={scannerType}
              onChange={e => setScannerType(e.target.value as ScannerType)}
              className="w-full px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]"
            >
              <option value="trivy">Trivy (Container Vulnerability)</option>
              <option value="sonarqube">SonarQube (Code Quality)</option>
              <option value="both">Both</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Target Type</label>
            <select
              value={targetType}
              onChange={e => setTargetType(e.target.value as TargetType)}
              className="w-full px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]"
            >
              <option value="image">Container Image</option>
              <option value="git_repo">Git Repository</option>
              <option value="filesystem">Filesystem</option>
              <option value="sonarqube_project">SonarQube Project</option>
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Target Reference</label>
            <input
              type="text"
              value={targetRef}
              onChange={e => setTargetRef(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]"
              placeholder={targetType === 'image' ? 'nginx:latest' : targetType === 'git_repo' ? 'https://github.com/...' : '/path/to/scan'}
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={creating || !name || !targetRef}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 text-sm"
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
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
      <div className="bg-[var(--bg-primary)] rounded-lg border border-[var(--border)] p-6 w-full max-w-md">
        <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-4">Create Scan Schedule</h2>
        
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Target</label>
            <select
              value={targetId}
              onChange={e => setTargetId(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)]"
            >
              <option value="">Select a target...</option>
              {targets.map(t => (
                <option key={t.id} value={t.id}>{t.name} ({t.target_ref})</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Schedule Preset</label>
            <div className="grid grid-cols-2 gap-2">
              {presets.map(p => (
                <button
                  key={p.value}
                  onClick={() => setCronExpression(p.value)}
                  className={`px-3 py-2 text-sm rounded border ${cronExpression === p.value ? 'bg-blue-500/10 border-blue-500/50 text-blue-500' : 'bg-[var(--bg-secondary)] border-[var(--border)] text-[var(--text-secondary)]'}`}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-[var(--text-secondary)] mb-1">Cron Expression</label>
            <input
              type="text"
              value={cronExpression}
              onChange={e => setCronExpression(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg text-[var(--text-primary)] font-mono"
              placeholder="0 2 * * *"
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={creating || !targetId || !cronExpression}
            className="px-4 py-2 bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 text-sm"
          >
            {creating ? 'Creating...' : 'Create Schedule'}
          </button>
        </div>
      </div>
    </div>
  );
}
