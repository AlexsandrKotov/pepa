'use client';

import { useState, useEffect, useCallback } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { clusters, environments, connections, Cluster, Environment } from '@/lib/api';
import { Modal, Toast } from '@/components/Interactive';
import ConceptHelp from '@/components/ConceptHelp';
import EmptyState from '@/components/EmptyState';
import ConfirmModal from '@/components/ConfirmModal';
import BrandIcon from '@/components/BrandIcon';

// ── Kubeconfig Parser ────────────────────────────────────────

interface ParsedCluster {
  name: string;
  server: string;
  kubeconfigSubset: string;
}

interface ClusterFormData {
  name: string;
  description: string;
  environment: string;
  apiServer: string;
  labelsStr: string;
  notes: string;
  kubeconfigSubset: string;
}

function parseKubeconfig(content: string): ParsedCluster[] {
  if (!content) return [];

  // Normalize tabs to spaces
  content = content.replace(/\t/g, '  ');

  // Find clusters, contexts, users sections using line-by-line approach
  const allLines = content.split('\n');

  const extractEntries = (sectionKey: string): string[] => {
    let inSection = false;
    let itemIndent = -1;
    const entries: string[] = [];
    let currentEntry: string[] = [];

    for (const line of allLines) {
      // Skip empty lines and comments at top level
      if (!inSection) {
        const trimmed = line.trim();
        if (trimmed === sectionKey || trimmed.startsWith(sectionKey)) {
          inSection = true;
        }
        continue;
      }

      // Check if we hit the next top-level key (end of section)
      // Top-level keys start with a letter (e.g. "contexts:", "apiVersion:"), not with "-"
      if (line.length > 0 && line[0].match(/[a-zA-Z]/) && line.trim() !== '' && !line.startsWith('#')) {
        break;
      }

      // Check if this is a list item (contains "- " after indentation)
      const dashMatch = line.match(/^(\s*)- (.*)$/);
      if (dashMatch) {
        const indent = dashMatch[1].length;
        if (itemIndent === -1) {
          itemIndent = indent; // First item sets the expected indent
        }
        if (indent === itemIndent) {
          // New top-level list item - save previous entry
          if (currentEntry.length > 0) {
            entries.push(currentEntry.join('\n'));
          }
          currentEntry = [line];
          continue;
        }
      }

      // Continuation of current entry
      if (currentEntry.length > 0) {
        currentEntry.push(line);
      }
    }

    // Don't forget the last entry
    if (currentEntry.length > 0) {
      entries.push(currentEntry.join('\n'));
    }

    return entries;
  };

  const clusterEntries = extractEntries('clusters:');

  // Parse contexts and users for building subset kubeconfigs
  const contextEntries = extractEntries('contexts:');
  const contexts: ParsedContext[] = contextEntries.map(entry => {
    const nameMatch = entry.match(/^\s*name:\s*(.+)/m);
    const clusterMatch = entry.match(/^\s*cluster:\s*(.+)/m);
    const userMatch = entry.match(/^\s*user:\s*(.+)/m);
    if (!nameMatch) return null;
    return {
      name: nameMatch[1].trim().replace(/^["']|["']$/g, ''),
      clusterRef: clusterMatch ? clusterMatch[1].trim().replace(/^["']|["']$/g, '') : '',
      userRef: userMatch ? userMatch[1].trim().replace(/^["']|["']$/g, '') : '',
    };
  }).filter(Boolean) as ParsedContext[];

  const userEntries = extractEntries('users:');
  const users: ParsedUser[] = userEntries.map(entry => {
    const nameMatch = entry.match(/^\s*name:\s*(.+)/m);
    if (!nameMatch) return null;
    return {
      name: nameMatch[1].trim().replace(/^["']|["']$/g, ''),
      block: entry.trim(),
    };
  }).filter(Boolean) as ParsedUser[];

  const results: ParsedCluster[] = [];

  for (const entry of clusterEntries) {
    const nameMatch = entry.match(/^\s*name:\s*(.+)/m);
    const serverMatch = entry.match(/\bserver:\s*(.+)/m);

    if (nameMatch) {
      const name = nameMatch[1].trim().replace(/^["']|["']$/g, '');
      const server = serverMatch ? serverMatch[1].trim().replace(/^["']|["']$/g, '') : '';

      // Find context for this cluster
      const ctx = contexts.find(c => c.clusterRef === name) ?? null;
      // Find user for this context
      const userEntry = ctx ? (users.find(u => u.name === ctx.userRef) ?? null) : null;

      // Build single-cluster kubeconfig
      const subset = buildSingleClusterKubeconfig(name, server, entry, ctx, userEntry, content);

      results.push({ name, server, kubeconfigSubset: subset });
    }
  }

  return results;
}

interface ParsedContext {
  name: string;
  clusterRef: string;
  userRef: string;
}

interface ParsedUser {
  name: string;
  block: string;
}

function buildSingleClusterKubeconfig(
  clusterName: string,
  server: string,
  clusterBlock: string,
  ctx: ParsedContext | null,
  userEntry: ParsedUser | null,
  _fullContent: string,
): string {
  // Strip leading "- " from extracted entries since they already include the YAML list dash
  const cleanedCluster = clusterBlock.trim().replace(/^-\s+/, '');

  const contextName = `${clusterName}-context`;
  const userName = userEntry?.name || `${clusterName}-user`;

  let yaml = `apiVersion: v1\nkind: Config\n`;
  yaml += `current-context: ${contextName}\n`;
  yaml += `clusters:\n- ${cleanedCluster}\n`;
  yaml += `contexts:\n- context:\n    cluster: ${clusterName}\n    user: ${userName}\n  name: ${contextName}\n`;

  if (userEntry) {
    const cleanedUser = userEntry.block.replace(/^-\s+/, '');
    yaml += `users:\n- ${cleanedUser}\n`;
  } else {
    yaml += `users:\n- name: ${userName}\n  user: {}\n`;
  }

  return yaml;
}

function inferEnvironment(name: string): string {
  const lower = name.toLowerCase();
  if (lower.includes('prod') || lower.includes('production')) return 'production';
  if (lower.includes('stag') || lower.includes('uat')) return 'staging';
  if (lower.includes('dev') || lower.includes('develop') || lower.includes('local') || lower.includes('test')) return 'dev';
  return 'dev';
}

// ── Main Component ───────────────────────────────────────────

export default function ClustersClient() {
  const [clusterList, setClusterList] = useState<Cluster[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [showImportKubeconfig, setShowImportKubeconfig] = useState(false);
  const [editCluster, setEditCluster] = useState<Cluster | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Cluster | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  // Escape key closes modals
  const anyModalOpen = showAdd || showImportKubeconfig || editCluster !== null;
  useEscapeKey(() => {
    if (showImportKubeconfig) setShowImportKubeconfig(false);
    else if (editCluster) setEditCluster(null);
    else if (showAdd) setShowAdd(false);
  }, anyModalOpen);

  const loadClusters = useCallback(async () => {
    try {
      const data = await clusters.list();
      setClusterList(data.clusters || []);
    } catch {
      setToast({ message: 'Failed to load clusters', type: 'error' });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadClusters(); }, [loadClusters]);

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await clusters.delete(deleteTarget.id);
      setClusterList(prev => prev.filter(c => c.id !== deleteTarget.id));
      setToast({ message: `Cluster "${deleteTarget.name}" deleted`, type: 'success' });
      setDeleteTarget(null);
    } catch {
      setToast({ message: 'Failed to delete cluster', type: 'error' });
    } finally {
      setDeleting(false);
    }
  };

  const envColors: Record<string, string> = {
    production: 'bg-red-500/15 text-red-500 border-red-500/20',
    staging: 'bg-amber-500/15 text-amber-600 border-amber-500/20',
    dev: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/20',
    development: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/20',
  };

  const statusDot: Record<string, string> = {
    connected: 'bg-green-500',
    disconnected: 'bg-red-500',
    syncing: 'bg-yellow-500',
    pending: 'bg-gray-400',
  };

  const statusBadge: Record<string, string> = {
    connected: 'badge-success',
    disconnected: 'badge-danger',
    syncing: 'badge-warning',
    pending: 'badge-default',
  };

  if (loading) {
    return (
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <h1 className="page-title-modern">Kubernetes Clusters</h1>
          <div className="card card-body text-center py-12" style={{ borderRadius: '12px' }}>
            <p className="text-[13px] text-[var(--text-tertiary)]">Loading clusters...</p>
          </div>
        </div>
      </div>
    );
  }

  const connected = clusterList.filter(c => c.status === 'connected').length;
  const fluxCount = clusterList.filter(c => c.flux_installed).length;
  const argoCount = clusterList.filter(c => c.labels?.argocd_detected === 'true').length;
  const gitopsCount = clusterList.filter(c => c.flux_installed || c.labels?.argocd_detected === 'true').length;
  const totalNodes = clusterList.reduce((sum, c) => sum + c.node_count, 0);

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Header */}
      <div className="page-animate">
        <div className="flex items-center justify-between">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="page-title-modern">Kubernetes Clusters</h1>
              <ConceptHelp term="cluster" />
            </div>
            <p className="page-subtitle-modern">Manage clusters via kubeconfig, monitor GitOps resources</p>
          </div>
          <div className="flex gap-2">
            <button onClick={() => setShowImportKubeconfig(true)} className="btn btn-secondary">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
              </svg>
              Import Kubeconfig
            </button>
            <button onClick={() => setShowAdd(true)} className="btn btn-primary">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
              </svg>
              Add Cluster
            </button>
          </div>
        </div>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-4 gap-4 page-animate-up page-delay-1">
        <div className="modern-stat-card flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl stat-icon-blue flex items-center justify-center text-white text-sm">
            <BrandIcon name="kubernetes" size={20} monochrome />
          </div>
          <div>
            <p className="text-[22px] font-bold text-[var(--text-primary)]">{clusterList.length}</p>
            <p className="text-[11px] text-[var(--text-tertiary)]">Total Clusters</p>
          </div>
        </div>
        <div className="modern-stat-card flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl stat-icon-green flex items-center justify-center text-white text-sm">
            <BrandIcon name="argocd" size={20} monochrome />
          </div>
          <div>
            <p className="text-[22px] font-bold text-emerald-600">{connected}</p>
            <p className="text-[11px] text-[var(--text-tertiary)]">Connected</p>
          </div>
        </div>
        <div className="modern-stat-card flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl stat-icon-purple flex items-center justify-center text-white text-sm">
            {fluxCount > 0 && argoCount === 0 ? (
              <BrandIcon name="fluxcd" size={20} monochrome />
            ) : argoCount > 0 && fluxCount === 0 ? (
              <BrandIcon name="argocd" size={20} monochrome />
            ) : (
              <BrandIcon name="gitops" size={20} monochrome />
            )}
          </div>
          <div>
            <p className="text-[22px] font-bold text-blue-500">{gitopsCount}</p>
            <p className="text-[11px] text-[var(--text-tertiary)]">
              {fluxCount > 0 && argoCount > 0 ? `GitOps (${fluxCount} Flux, ${argoCount} Argo)` : fluxCount > 0 ? 'FluxCD' : argoCount > 0 ? 'ArgoCD' : 'GitOps'}
            </p>
          </div>
        </div>
        <div className="modern-stat-card flex items-center gap-3">
          <div className="w-10 h-10 rounded-xl stat-icon-amber flex items-center justify-center text-white text-sm">
            <BrandIcon name="dashboard" size={20} monochrome />
          </div>
          <div>
            <p className="text-[22px] font-bold text-[var(--text-primary)]">{totalNodes}</p>
            <p className="text-[11px] text-[var(--text-tertiary)]">Total Nodes</p>
          </div>
        </div>
      </div>

      {/* Cluster List */}
      <div className="space-y-3 page-animate-up page-delay-2">
        {clusterList.map((cluster) => (
          <div
            key={cluster.id}
            className="block card p-4 modern-card-hover"
            style={{ borderRadius: '12px' }}
          >
            <div className="flex items-center justify-between">
              <Link
                href={`/clusters?id=${cluster.id}`}
                className="flex items-center gap-3 flex-1 min-w-0"
              >
                <div className={`w-2.5 h-2.5 rounded-full shrink-0 ${statusDot[cluster.status] || 'bg-gray-400'}`} />
                <div className="min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-[14px] font-medium text-[var(--text-primary)]">{cluster.name}</span>
                    <span className={`badge ${statusBadge[cluster.status] || 'badge-default'}`}>
                      {cluster.status}
                    </span>
                    <span className={`text-[11px] px-1.5 py-0.5 rounded border ${envColors[cluster.environment] || 'bg-[var(--border-light)] text-[var(--text-secondary)] border-[var(--border)]'}`}>
                      {cluster.environment}
                    </span>
                    {cluster.has_kubeconfig && (
                      <span className="text-[11px] px-1.5 py-0.5 rounded bg-blue-500/10 text-blue-500 border border-blue-500/20">
                        kubeconfig
                      </span>
                    )}
                    {cluster.flux_installed && (
                      <span className="text-[11px] px-1.5 py-0.5 rounded bg-purple-500/10 text-purple-500 border border-purple-500/20">
                        FluxCD
                      </span>
                    )}
                    {cluster.labels?.argocd_detected === 'true' && (
                      <span className="text-[11px] px-1.5 py-0.5 rounded bg-orange-500/10 text-orange-500 border border-orange-500/20">
                        ArgoCD
                      </span>
                    )}
                  </div>
                  {cluster.description && (
                    <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5 truncate max-w-md">{cluster.description}</p>
                  )}
                  <div className="text-[12px] text-[var(--text-tertiary)] mt-1">
                    {cluster.api_server_url || 'No API server'} &middot; {cluster.kubernetes_version || 'N/A'} &middot; {cluster.node_count} nodes
                  </div>
                </div>
              </Link>
              <div className="flex items-center gap-2 shrink-0 ml-3">
                <button
                  onClick={() => setEditCluster(cluster)}
                  className="p-1.5 rounded hover:bg-[var(--border-light)] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] transition-colors"
                  title="Edit cluster"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
                  </svg>
                </button>
                <button
                  onClick={() => setDeleteTarget(cluster)}
                  className="p-1.5 rounded hover:bg-red-500/10 text-[var(--text-tertiary)] hover:text-red-500 transition-colors"
                  title="Delete cluster"
                >
                  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.7 51.7 0 00-3.382 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                  </svg>
                </button>
                <Link href={`/clusters?id=${cluster.id}`}>
                  <svg className="w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                  </svg>
                </Link>
              </div>
            </div>
          </div>
        ))}
        {clusterList.length === 0 && (
          <>
            <EmptyState
              icon={<BrandIcon name="kubernetes" size={48} />}
              title="No clusters connected"
              description="A cluster is a Kubernetes environment where your services run. Connect one by pasting its kubeconfig — PEPA will detect its health and GitOps engine automatically."
              actionLabel="+ Add Cluster"
              actionOnClick={() => setShowAdd(true)}
              secondaryHref="/setup"
              secondaryLabel="Open setup wizard"
            />
            <div className="text-center mt-3">
              <button
                onClick={() => setShowImportKubeconfig(true)}
                className="text-[12px] text-[var(--accent)] hover:underline"
              >
                Have a kubeconfig with multiple clusters? Import them all at once {'\u2192'}
              </button>
            </div>
          </>
        )}
      </div>

      {/* Import Kubeconfig Modal */}
      {showImportKubeconfig && <ImportKubeconfigModal onClose={() => setShowImportKubeconfig(false)} onCreated={loadClusters} />}

      {/* Add Cluster Modal */}
      {showAdd && <AddClusterModal onClose={() => setShowAdd(false)} onCreated={loadClusters} />}

      {/* Edit Cluster Modal */}
      {editCluster && (
        <EditClusterModal
          cluster={editCluster}
          onClose={() => setEditCluster(null)}
          onUpdated={() => { setEditCluster(null); loadClusters(); }}
        />
      )}

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteTarget}
        title="Delete Cluster"
        description={`Are you sure you want to delete "${deleteTarget?.name}"? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
      </div>
    </div>
  );
}

// ── Import Kubeconfig Modal ──────────────────────────────────

function ImportKubeconfigModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  useEscapeKey(onClose);
  const [kubeconfig, setKubeconfig] = useState('');
  const [parsedClusters, setParsedClusters] = useState<ParsedCluster[]>([]);
  const [clusterForms, setClusterForms] = useState<ClusterFormData[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [step, setStep] = useState<'input' | 'review'>('input');
  const [envOptions, setEnvOptions] = useState<Environment[]>([]);

  useEffect(() => {
    environments.list().then(data => {
      const envs = data.environments || [];
      setEnvOptions(envs);
    }).catch(() => {});
  }, []);

  const handleKubeconfigInput = useCallback((content: string) => {
    setKubeconfig(content);
    setError('');
    if (content.trim()) {
      const parsed = parseKubeconfig(content);
      setParsedClusters(parsed);
      if (parsed.length > 0) {
        const forms = parsed.map(pc => ({
          name: pc.name,
          description: '',
          environment: inferEnvironment(pc.name),
          apiServer: pc.server,
          labelsStr: '',
          notes: '',
          kubeconfigSubset: pc.kubeconfigSubset,
        }));
        setClusterForms(forms);
      } else {
        setClusterForms([]);
      }
    } else {
      setParsedClusters([]);
      setClusterForms([]);
    }
  }, []);

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (ev) => {
        const content = ev.target?.result as string;
        handleKubeconfigInput(content);
      };
      reader.readAsText(file);
    }
  };

  const updateClusterForm = (idx: number, field: keyof ClusterFormData, value: string) => {
    setClusterForms(prev => prev.map((form, i) => i === idx ? { ...form, [field]: value } : form));
  };

  const removeClusterForm = (idx: number) => {
    setClusterForms(prev => prev.filter((_, i) => i !== idx));
  };

  const parseLabels = (str: string): Record<string, string> => {
    const labels: Record<string, string> = {};
    str.split(',').forEach(pair => {
      const [key, val] = pair.split(':').map(s => s.trim());
      if (key && val) labels[key] = val;
    });
    return labels;
  };

  const handleSubmit = async () => {
    if (clusterForms.length === 0) { setError('Add at least one cluster'); return; }
    const emptyNames = clusterForms.filter(f => !f.name.trim());
    if (emptyNames.length > 0) { setError('All clusters must have a name'); return; }
    setLoading(true);
    setError('');
    try {
      // Use backend parser for reliable credential extraction
      let backendClusters: { name: string; server: string; kubeconfig: string }[] = [];
      if (kubeconfig.trim()) {
        try {
          const result = await connections.parseKubeconfig(kubeconfig);
          backendClusters = result.clusters || [];
        } catch { /* fallback to frontend-parsed subsets */ }
      }
      for (const form of clusterForms) {
        const cluster = await clusters.create({
          name: form.name,
          description: form.description,
          environment: form.environment,
          api_server_url: form.apiServer,
          flux_installed: false,
          status: 'pending',
          node_count: 0,
          kubernetes_version: '',
          labels: form.labelsStr ? parseLabels(form.labelsStr) : {},
          notes: form.notes,
          is_active: true,
        });
        // Find matching cluster from backend parser, fallback to frontend subset
        const backendMatch = backendClusters.find(c => c.name === form.name);
        await clusters.uploadKubeconfig(cluster.id, backendMatch?.kubeconfig || form.kubeconfigSubset);
      }
      onCreated();
      onClose();
    } catch {
      setError('Failed to create one or more clusters');
    } finally {
      setLoading(false);
    }
  };

  // Step 2: Review & edit parsed clusters
  if (step === 'review') {
    return (
      <Modal open={true} title={`Import Clusters (${clusterForms.length})`} onClose={onClose}>
        <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
          {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}

          <div className="p-3 bg-blue-500/10 border border-blue-500/20 rounded-lg">
            <p className="text-[13px] font-medium text-blue-500">
              Parsed {parsedClusters.length} cluster{parsedClusters.length !== 1 ? 's' : ''} from kubeconfig
            </p>
            <p className="text-[12px] text-[var(--text-secondary)] mt-1">
              Review and edit each cluster before importing. Each cluster will be created with its own kubeconfig subset.
            </p>
          </div>

          <div className="space-y-4">
            {clusterForms.map((form, idx) => (
              <div key={idx} className="p-4 rounded-lg border border-[var(--border)] space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="w-6 h-6 rounded-full bg-[var(--accent)] text-white text-[11px] font-medium flex items-center justify-center">{idx + 1}</span>
                    <span className="text-[13px] font-medium text-[var(--text-primary)]">{form.name || `Cluster ${idx + 1}`}</span>
                    <span className={`text-[11px] px-1.5 py-0.5 rounded border ${
                      inferEnvironment(form.name) === 'production' ? 'bg-red-500/15 text-red-500 border-red-500/20' :
                      inferEnvironment(form.name) === 'staging' ? 'bg-amber-500/15 text-amber-600 border-amber-500/20' :
                      'bg-emerald-500/15 text-emerald-600 border-emerald-500/20'
                    }`}>
                      {inferEnvironment(form.name)}
                    </span>
                  </div>
                  {clusterForms.length > 1 && (
                    <button
                      onClick={() => removeClusterForm(idx)}
                      className="p-1 rounded hover:bg-red-500/10 text-[var(--text-tertiary)] hover:text-red-500 transition-colors"
                      title="Skip this cluster"
                    >
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="label">Cluster Name *</label>
                    <input
                      type="text"
                      value={form.name}
                      onChange={(e) => updateClusterForm(idx, 'name', e.target.value)}
                      className="input"
                      placeholder="production-east"
                    />
                  </div>
                  <div>
                    <label className="label">Environment</label>
                    <select
                      value={form.environment}
                      onChange={(e) => updateClusterForm(idx, 'environment', e.target.value)}
                      className="select"
                    >
                      {envOptions.length === 0 && (
                        <>
                          <option value="dev">Development</option>
                          <option value="staging">Staging</option>
                          <option value="production">Production</option>
                        </>
                      )}
                      {envOptions.map(env => (
                        <option key={env.id} value={env.slug || env.name}>
                          {env.name}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>

                <div>
                  <label className="label">Description</label>
                  <input
                    type="text"
                    value={form.description}
                    onChange={(e) => updateClusterForm(idx, 'description', e.target.value)}
                    className="input"
                    placeholder="Cluster description..."
                  />
                </div>

                <div>
                  <label className="label">API Server URL</label>
                  <input
                    type="text"
                    value={form.apiServer}
                    onChange={(e) => updateClusterForm(idx, 'apiServer', e.target.value)}
                    className="input font-mono text-[12px]"
                    placeholder="https://k8s.example.com:6443"
                  />
                </div>

                <div>
                  <label className="label">Labels</label>
                  <input
                    type="text"
                    value={form.labelsStr}
                    onChange={(e) => updateClusterForm(idx, 'labelsStr', e.target.value)}
                    className="input"
                    placeholder="region:eu-west, team:platform"
                  />
                </div>

                <div>
                  <label className="label">Notes</label>
                  <textarea
                    value={form.notes}
                    onChange={(e) => updateClusterForm(idx, 'notes', e.target.value)}
                    placeholder="Internal notes..."
                    rows={2}
                    className="input resize-none text-[12px]"
                  />
                </div>
              </div>
            ))}
          </div>

          <div className="flex justify-between items-center pt-2">
            <button
              onClick={() => setStep('input')}
              className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
            >
              {'\u2190'} Back to kubeconfig
            </button>
            <div className="flex gap-2">
              <button onClick={onClose} className="btn btn-ghost">Cancel</button>
              <button
                onClick={handleSubmit}
                disabled={loading || clusterForms.length === 0}
                className="btn btn-primary"
              >
                {loading ? 'Importing...' : `Import ${clusterForms.length} Cluster${clusterForms.length !== 1 ? 's' : ''}`}
              </button>
            </div>
          </div>
        </div>
      </Modal>
    );
  }

  // Step 1: Paste or upload kubeconfig
  return (
    <Modal open={true} title="Import Clusters from Kubeconfig" onClose={onClose}>
      <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
        {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}

        <div className="p-3 bg-[var(--border-light)] border border-[var(--border)] rounded-lg">
          <p className="text-[13px] text-[var(--text-secondary)]">
            Paste a kubeconfig with one or more clusters. Each cluster will be extracted and created as a separate entry that you can edit before importing.
          </p>
        </div>

        <div className="space-y-2">
          <label className="label">Kubeconfig File</label>
          <input
            type="file"
            accept=".yaml,.yml,.conf,.kubeconfig,.config"
            onChange={handleFileUpload}
            className="w-full px-3 py-2 border border-[var(--border)] rounded-[6px] text-[13px] text-[var(--text-primary)] file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:bg-[var(--border-light)] file:text-[var(--text-secondary)] file:text-[12px] file:cursor-pointer"
          />
        </div>

        <div className="space-y-2">
          <label className="label">Or paste kubeconfig</label>
          <textarea
            value={kubeconfig}
            onChange={(e) => handleKubeconfigInput(e.target.value)}
            placeholder={'apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://...\n  name: my-cluster\ncontexts:\n- context:\n    cluster: my-cluster\n    user: my-user\n  name: my-context\nusers:\n- name: my-user\n  user: {}'}
            rows={10}
            className="input font-mono text-[12px] resize-none"
          />
        </div>

        {parsedClusters.length > 0 && (
          <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-lg">
            <p className="text-[13px] font-medium text-emerald-600 flex items-center gap-1.5">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              Found {parsedClusters.length} cluster{parsedClusters.length !== 1 ? 's' : ''}
            </p>
            <div className="mt-2 space-y-1">
              {parsedClusters.map((pc, idx) => (
                <div key={idx} className="flex items-center gap-2 text-[12px] text-emerald-600">
                  <span className="w-4 h-4 rounded-full bg-emerald-500/20 text-emerald-600 text-[10px] font-medium flex items-center justify-center shrink-0">{idx + 1}</span>
                  <span className="font-medium">{pc.name}</span>
                  <span className="text-green-500">{pc.server}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        <div className="flex justify-end gap-2 pt-2">
          <button onClick={onClose} className="btn btn-ghost">Cancel</button>
          <button
            onClick={() => setStep('review')}
            disabled={parsedClusters.length === 0}
            className="btn btn-primary"
          >
            Review &amp; Edit {'\u2192'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

// ── Add Cluster Modal ────────────────────────────────────────

function AddClusterModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  useEscapeKey(onClose);
  const [kubeconfig, setKubeconfig] = useState('');
  const [parsedClusters, setParsedClusters] = useState<ParsedCluster[]>([]);
  const [clusterForms, setClusterForms] = useState<ClusterFormData[]>([]);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [environment, setEnvironment] = useState('');
  const [apiServer, setApiServer] = useState('');
  const [labelsStr, setLabelsStr] = useState('');
  const [notes, setNotes] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [step, setStep] = useState<'input' | 'multi'>('input');
  const [envOptions, setEnvOptions] = useState<Environment[]>([]);

  useEffect(() => {
    environments.list().then(data => {
      const envs = data.environments || [];
      setEnvOptions(envs);
      // Set default to first env if available and current value is empty
      if (envs.length > 0 && !environment) {
        setEnvironment(envs[0].slug || envs[0].name);
      }
    }).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleKubeconfigChange = useCallback((content: string) => {
    setKubeconfig(content);
    if (content.trim()) {
      const parsed = parseKubeconfig(content);
      setParsedClusters(parsed);
      if (parsed.length > 1) {
        // Multi-cluster: initialize form data for each cluster
        const forms = parsed.map(pc => ({
          name: pc.name,
          description: '',
          environment: inferEnvironment(pc.name),
          apiServer: pc.server,
          labelsStr: '',
          notes: '',
          kubeconfigSubset: pc.kubeconfigSubset,
        }));
        setClusterForms(forms);
        setStep('multi');
      } else if (parsed.length === 1) {
        // Single cluster: auto-fill fields
        setName(parsed[0].name);
        setApiServer(parsed[0].server);
        setEnvironment(inferEnvironment(parsed[0].name));
        setStep('input');
      } else {
        setStep('input');
      }
    } else {
      setParsedClusters([]);
      setClusterForms([]);
      setStep('input');
    }
  }, []);

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      const reader = new FileReader();
      reader.onload = (ev) => {
        const content = ev.target?.result as string;
        handleKubeconfigChange(content);
      };
      reader.readAsText(file);
    }
  };

  const updateClusterForm = (idx: number, field: keyof ClusterFormData, value: string) => {
    setClusterForms(prev => prev.map((form, i) => i === idx ? { ...form, [field]: value } : form));
  };

  const removeClusterForm = (idx: number) => {
    setClusterForms(prev => prev.filter((_, i) => i !== idx));
  };

  const parseLabels = (str: string): Record<string, string> => {
    const labels: Record<string, string> = {};
    str.split(',').forEach(pair => {
      const [key, val] = pair.split(':').map(s => s.trim());
      if (key && val) labels[key] = val;
    });
    return labels;
  };

  const handleSubmitSingle = async () => {
    if (!name) { setError('Name is required'); return; }
    if (!kubeconfig) { setError('Kubeconfig is required'); return; }
    setLoading(true);
    setError('');
    try {
      // Use backend parser for reliable credential extraction
      let kubeconfigToUpload = kubeconfig;
      try {
        const result = await connections.parseKubeconfig(kubeconfig);
        const match = result.clusters?.find(c => c.name === name) || result.clusters?.[0];
        if (match) kubeconfigToUpload = match.kubeconfig;
      } catch { /* fallback to raw kubeconfig */ }

      const cluster = await clusters.create({
        name,
        description,
        environment,
        api_server_url: apiServer,
        flux_installed: false,
        status: 'pending',
        node_count: 0,
        kubernetes_version: '',
        labels: labelsStr ? parseLabels(labelsStr) : {},
        notes,
        is_active: true,
      });
      await clusters.uploadKubeconfig(cluster.id, kubeconfigToUpload);
      onCreated();
      onClose();
    } catch {
      setError('Failed to create cluster');
    } finally {
      setLoading(false);
    }
  };

  const handleSubmitMulti = async () => {
    if (clusterForms.length === 0) { setError('Add at least one cluster'); return; }
    // Validate all forms have names
    const emptyNames = clusterForms.filter(f => !f.name.trim());
    if (emptyNames.length > 0) { setError('All clusters must have a name'); return; }
    setLoading(true);
    setError('');
    try {
      // Use backend parser for reliable credential extraction
      let backendClusters: { name: string; server: string; kubeconfig: string }[] = [];
      if (kubeconfig.trim()) {
        try {
          const result = await connections.parseKubeconfig(kubeconfig);
          backendClusters = result.clusters || [];
        } catch { /* fallback to frontend-parsed subsets */ }
      }
      for (const form of clusterForms) {
        const cluster = await clusters.create({
          name: form.name,
          description: form.description,
          environment: form.environment,
          api_server_url: form.apiServer,
          flux_installed: false,
          status: 'pending',
          node_count: 0,
          kubernetes_version: '',
          labels: form.labelsStr ? parseLabels(form.labelsStr) : {},
          notes: form.notes,
          is_active: true,
        });
        const backendMatch = backendClusters.find(c => c.name === form.name);
        await clusters.uploadKubeconfig(cluster.id, backendMatch?.kubeconfig || form.kubeconfigSubset);
      }
      onCreated();
      onClose();
    } catch {
      setError('Failed to create one or more clusters');
    } finally {
      setLoading(false);
    }
  };

  // Multi-cluster form step
  if (step === 'multi') {
    return (
      <Modal open={true} title={`Add Clusters (${clusterForms.length})`} onClose={onClose}>
        <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
          {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}

          <div className="p-3 bg-blue-500/10 border border-blue-500/20 rounded-lg">
            <p className="text-[13px] font-medium text-blue-500">
              Detected {parsedClusters.length} clusters in kubeconfig
            </p>
            <p className="text-[12px] text-[var(--text-secondary)] mt-1">
              Fill in details for each cluster. Each will be created as a separate cluster entry with its own kubeconfig.
            </p>
          </div>

          <div className="space-y-4">
            {clusterForms.map((form, idx) => (
              <div key={idx} className="p-4 rounded-lg border border-[var(--border)] space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="w-6 h-6 rounded-full bg-[var(--accent)] text-white text-[11px] font-medium flex items-center justify-center">{idx + 1}</span>
                    <span className="text-[13px] font-medium text-[var(--text-primary)]">{form.name || `Cluster ${idx + 1}`}</span>
                    <span className={`text-[11px] px-1.5 py-0.5 rounded border ${
                      inferEnvironment(form.name) === 'production' ? 'bg-red-500/15 text-red-500 border-red-500/20' :
                      inferEnvironment(form.name) === 'staging' ? 'bg-amber-500/15 text-amber-600 border-amber-500/20' :
                      'bg-emerald-500/15 text-emerald-600 border-emerald-500/20'
                    }`}>
                      {inferEnvironment(form.name)}
                    </span>
                  </div>
                  {clusterForms.length > 1 && (
                    <button
                      onClick={() => removeClusterForm(idx)}
                      className="p-1 rounded hover:bg-red-500/10 text-[var(--text-tertiary)] hover:text-red-500 transition-colors"
                      title="Remove this cluster"
                    >
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                      </svg>
                    </button>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="label">Cluster Name *</label>
                    <input
                      type="text"
                      value={form.name}
                      onChange={(e) => updateClusterForm(idx, 'name', e.target.value)}
                      className="input"
                      placeholder="production-east"
                    />
                  </div>
                  <div>
                    <label className="label">Environment</label>
                    <select
                      value={form.environment}
                      onChange={(e) => updateClusterForm(idx, 'environment', e.target.value)}
                      className="select"
                    >
                      {envOptions.length === 0 && (
                        <>
                          <option value="dev">Development</option>
                          <option value="staging">Staging</option>
                          <option value="production">Production</option>
                        </>
                      )}
                      {envOptions.map(env => (
                        <option key={env.id} value={env.slug || env.name}>
                          {env.name}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>

                <div>
                  <label className="label">Description</label>
                  <input
                    type="text"
                    value={form.description}
                    onChange={(e) => updateClusterForm(idx, 'description', e.target.value)}
                    className="input"
                    placeholder="Cluster description..."
                  />
                </div>

                <div>
                  <label className="label">API Server URL</label>
                  <input
                    type="text"
                    value={form.apiServer}
                    onChange={(e) => updateClusterForm(idx, 'apiServer', e.target.value)}
                    className="input font-mono text-[12px]"
                    placeholder="https://k8s.example.com:6443"
                  />
                </div>

                <div>
                  <label className="label">Labels</label>
                  <input
                    type="text"
                    value={form.labelsStr}
                    onChange={(e) => updateClusterForm(idx, 'labelsStr', e.target.value)}
                    className="input"
                    placeholder="region:eu-west, team:platform"
                  />
                </div>

                <div>
                  <label className="label">Notes</label>
                  <textarea
                    value={form.notes}
                    onChange={(e) => updateClusterForm(idx, 'notes', e.target.value)}
                    placeholder="Internal notes..."
                    rows={2}
                    className="input resize-none text-[12px]"
                  />
                </div>
              </div>
            ))}
          </div>

          <div className="flex justify-between items-center pt-2">
            <button
              onClick={() => { setStep('input'); setParsedClusters([]); setClusterForms([]); setKubeconfig(''); }}
              className="text-[12px] text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]"
            >
              {'\u2190'} Back to input
            </button>
            <div className="flex gap-2">
              <button onClick={onClose} className="btn btn-ghost">Cancel</button>
              <button
                onClick={handleSubmitMulti}
                disabled={loading || clusterForms.length === 0}
                className="btn btn-primary"
              >
                {loading ? 'Adding...' : `Add ${clusterForms.length} Cluster${clusterForms.length !== 1 ? 's' : ''}`}
              </button>
            </div>
          </div>
        </div>
      </Modal>
    );
  }

  // Single cluster / manual input step
  return (
    <Modal open={true} title="Add Kubernetes Cluster" onClose={onClose}>
      <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
        {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}

        {/* Kubeconfig Section */}
        <div className="space-y-2">
          <label className="label">Kubeconfig</label>
          <input
            type="file"
            accept=".yaml,.yml,.conf,.kubeconfig,.config"
            onChange={handleFileUpload}
            className="w-full px-3 py-2 border border-[var(--border)] rounded-[6px] text-[13px] text-[var(--text-primary)] file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:bg-[var(--border-light)] file:text-[var(--text-secondary)] file:text-[12px] file:cursor-pointer"
          />
          <textarea
            value={kubeconfig}
            onChange={(e) => handleKubeconfigChange(e.target.value)}
            placeholder={'apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://...'}
            rows={5}
            className="input font-mono text-[12px] resize-none"
          />
          {parsedClusters.length === 1 && (
            <p className="text-[11px] text-green-600 flex items-center gap-1">
              <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              Parsed cluster: {parsedClusters[0].name} ({parsedClusters[0].server})
            </p>
          )}
          {parsedClusters.length > 1 && (
            <button
              onClick={() => {
                const forms = parsedClusters.map(pc => ({
                  name: pc.name,
                  description: '',
                  environment: inferEnvironment(pc.name),
                  apiServer: pc.server,
                  labelsStr: '',
                  notes: '',
                  kubeconfigSubset: pc.kubeconfigSubset,
                }));
                setClusterForms(forms);
                setStep('multi');
              }}
              className="text-[12px] text-[var(--accent)] hover:underline"
            >
              {parsedClusters.length} clusters detected — review selection {'\u2192'}
            </button>
          )}
        </div>

        {/* Name & Environment */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="label">Cluster Name *</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="input"
              placeholder="production-east"
            />
          </div>
          <div>
            <label className="label">Environment</label>
            <select
              value={environment}
              onChange={(e) => setEnvironment(e.target.value)}
              className="select"
            >
              {envOptions.length === 0 && (
                <>
                  <option value="dev">Development</option>
                  <option value="staging">Staging</option>
                  <option value="production">Production</option>
                </>
              )}
              {envOptions.map(env => (
                <option key={env.id} value={env.slug || env.name}>
                  {env.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Description */}
        <div>
          <label className="label">Description</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="input"
            placeholder="Main production cluster in EU region"
          />
        </div>

        {/* API Server */}
        <div>
          <label className="label">API Server URL</label>
          <input
            type="text"
            value={apiServer}
            onChange={(e) => setApiServer(e.target.value)}
            className="input"
            placeholder="https://k8s.example.com:6443"
          />
          <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Auto-detected from kubeconfig</p>
        </div>

        {/* Labels */}
        <div>
          <label className="label">Labels</label>
          <input
            type="text"
            value={labelsStr}
            onChange={(e) => setLabelsStr(e.target.value)}
            className="input"
            placeholder="region:eu-west, team:platform, tier:production"
          />
          <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Comma-separated key:value pairs</p>
        </div>

        {/* Notes */}
        <div>
          <label className="label">Notes</label>
          <textarea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Internal notes about this cluster..."
            rows={3}
            className="input resize-none"
          />
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-2">
          <button onClick={onClose} className="btn btn-ghost">Cancel</button>
          <button
            onClick={handleSubmitSingle}
            disabled={loading}
            className="btn btn-primary"
          >
            {loading ? 'Connecting...' : 'Connect Cluster'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

// ── Edit Cluster Modal ───────────────────────────────────────

function EditClusterModal({ cluster, onClose, onUpdated }: {
  cluster: Cluster;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const [name, setName] = useState(cluster.name);
  const [description, setDescription] = useState(cluster.description || '');
  const [environment, setEnvironment] = useState(cluster.environment);
  const [apiServer, setApiServer] = useState(cluster.api_server_url || '');
  const [labelsStr, setLabelsStr] = useState(
    Object.entries(cluster.labels || {}).map(([k, v]) => `${k}:${v}`).join(', ')
  );
  const [notes, setNotes] = useState(cluster.notes || '');
  const [kubeconfig, setKubeconfig] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [envOptions, setEnvOptions] = useState<Environment[]>([]);

  useEffect(() => {
    environments.list().then(data => {
      setEnvOptions(data.environments || []);
    }).catch(() => {});
  }, []);

  const parseLabels = (str: string): Record<string, string> => {
    const labels: Record<string, string> = {};
    str.split(',').forEach(pair => {
      const [key, val] = pair.split(':').map(s => s.trim());
      if (key && val) labels[key] = val;
    });
    return labels;
  };

  const handleSubmit = async () => {
    if (!name.trim()) { setError('Name is required'); return; }
    setLoading(true);
    setError('');
    try {
      await clusters.update(cluster.id, {
        name: name.trim(),
        description,
        environment,
        api_server_url: apiServer,
        labels: labelsStr ? parseLabels(labelsStr) : {},
        notes,
      });
      // If kubeconfig was provided, upload it
      if (kubeconfig.trim()) {
        try {
          // Use backend parser (proper Go YAML parser) for reliable credential extraction
          const result = await connections.parseKubeconfig(kubeconfig);
          if (result.clusters && result.clusters.length > 0) {
            // Find the cluster matching this cluster's name, or use the first one
            const match = result.clusters.find(c => c.name === cluster.name) || result.clusters[0];
            await clusters.uploadKubeconfig(cluster.id, match.kubeconfig);
          } else {
            // Fallback: send raw kubeconfig
            await clusters.uploadKubeconfig(cluster.id, kubeconfig);
          }
        } catch {
          // If backend parsing fails, send raw kubeconfig
          await clusters.uploadKubeconfig(cluster.id, kubeconfig);
        }
      }
      onUpdated();
    } catch {
      setError('Failed to update cluster');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Modal open={true} title={`Edit Cluster: ${cluster.name}`} onClose={onClose}>
      <div className="space-y-4 max-h-[70vh] overflow-y-auto pr-1">
        {error && <div className="p-2 bg-red-500/10 border border-red-500/20 rounded text-red-500 text-[13px]">{error}</div>}

        {/* Name & Environment */}
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="label">Cluster Name *</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="input"
              placeholder="production-east"
            />
          </div>
          <div>
            <label className="label">Environment</label>
            <select
              value={environment}
              onChange={(e) => setEnvironment(e.target.value)}
              className="select"
            >
              {envOptions.length === 0 && (
                <>
                  <option value="dev">Development</option>
                  <option value="staging">Staging</option>
                  <option value="production">Production</option>
                </>
              )}
              {envOptions.map(env => (
                <option key={env.id} value={env.slug || env.name}>
                  {env.name}
                </option>
              ))}
            </select>
          </div>
        </div>

        {/* Description */}
        <div>
          <label className="label">Description</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="input"
            placeholder="Cluster description..."
          />
        </div>

        {/* API Server */}
        <div>
          <label className="label">API Server URL</label>
          <input
            type="text"
            value={apiServer}
            onChange={(e) => setApiServer(e.target.value)}
            className="input"
            placeholder="https://k8s.example.com:6443"
          />
        </div>

        {/* Labels */}
        <div>
          <label className="label">Labels</label>
          <input
            type="text"
            value={labelsStr}
            onChange={(e) => setLabelsStr(e.target.value)}
            className="input"
            placeholder="region:eu-west, team:platform"
          />
          <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Comma-separated key:value pairs</p>
        </div>

        {/* Notes */}
        <div>
          <label className="label">Notes</label>
          <textarea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            placeholder="Internal notes..."
            rows={3}
            className="input resize-none"
          />
        </div>

        {/* Kubeconfig */}
        <div>
          <label className="label">Kubeconfig (optional)</label>
          <p className="text-[11px] text-[var(--text-tertiary)] mb-1">Re-upload to update credentials. Leave empty to keep current.</p>
          <input
            type="file"
            accept=".yaml,.yml,.conf,.kubeconfig,.config"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) {
                const reader = new FileReader();
                reader.onload = (ev) => {
                  setKubeconfig(ev.target?.result as string);
                };
                reader.readAsText(file);
              }
            }}
            className="w-full px-3 py-2 border border-[var(--border)] rounded-[6px] text-[13px] text-[var(--text-primary)] file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:bg-[var(--border-light)] file:text-[var(--text-secondary)] file:text-[12px] file:cursor-pointer"
          />
          <textarea
            value={kubeconfig}
            onChange={(e) => setKubeconfig(e.target.value)}
            placeholder="Or paste kubeconfig here..."
            rows={4}
            className="input font-mono text-[12px] resize-none mt-2"
          />
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 pt-2">
          <button onClick={onClose} className="btn btn-ghost">Cancel</button>
          <button
            onClick={handleSubmit}
            disabled={loading}
            className="btn btn-primary"
          >
            {loading ? 'Saving...' : 'Save Changes'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
