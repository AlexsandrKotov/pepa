'use client';

import { useState, useEffect, useRef, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import BrandIcon from '@/components/BrandIcon';
import { getBase } from '@/lib/api';

interface PaletteItem {
  id: string;
  label: string;
  description?: string;
  href?: string;
  action?: () => void;
  icon: string;
  category: string;
}

// All navigable pages — categories mirror the sidebar grouping
const pages: PaletteItem[] = [
  { id: 'dashboard', label: 'Dashboard', description: 'Platform overview', href: '/', icon: 'dashboard', category: 'Core' },
  { id: 'deploy', label: 'Quick Deploy', description: 'Deploy a service wizard', href: '/deploy', icon: 'argocd', category: 'Core' },
  { id: 'ai', label: 'AI Assistant', description: 'Chat with AI', href: '/ai', icon: 'ai', category: 'Core' },
  { id: 'services', label: 'Services', description: 'Manage services', href: '/services', icon: 'storage', category: 'Catalog' },
  { id: 'entities', label: 'Entities', description: 'Entity management', href: '/entities', icon: 'plugin', category: 'Catalog' },
  { id: 'scorecards', label: 'Scorecards', description: 'Maturity scorecards', href: '/scorecards', icon: 'plugin', category: 'Catalog' },
  { id: 'discovery', label: 'Discovery', description: 'Service discovery', href: '/discovery', icon: 'discovery', category: 'Catalog' },
  { id: 'import', label: 'Import', description: 'Import from GitLab/GitHub', href: '/import', icon: 'gitlab', category: 'Catalog' },
  { id: 'pipelines', label: 'Pipelines', description: 'CI/CD pipeline sources and providers', href: '/pipelines', icon: 'cicd', category: 'Delivery' },
  { id: 'pipeline-builder', label: 'Pipeline Builder', description: 'Compose deployment pipelines', href: '/pipeline-builder', icon: 'plugin', category: 'Delivery' },
  { id: 'pipeline-blueprints', label: 'Blueprints', description: 'Service blueprints for pipelines', href: '/pipeline-blueprints', icon: 'plugin', category: 'Delivery' },
  { id: 'cicd', label: 'CI/CD Providers', description: 'CI/CD system connections', href: '/pipelines?tab=providers', icon: 'cicd', category: 'Delivery' },
  { id: 'deployments', label: 'Deployments', description: 'GitOps deployments', href: '/deployments', icon: 'argocd', category: 'Delivery' },
  { id: 'environments', label: 'Environments', description: 'Deployment environments', href: '/environments', icon: 'discovery', category: 'Delivery' },
  { id: 'gitops', label: 'GitOps', description: 'GitOps overview', href: '/gitops', icon: 'fluxcd', category: 'Delivery' },
  { id: 'clusters', label: 'Clusters', description: 'Kubernetes clusters', href: '/clusters', icon: 'kubernetes', category: 'Infrastructure' },
  { id: 'docker-hosts', label: 'Docker Hosts', description: 'Docker host management', href: '/docker-hosts', icon: 'docker', category: 'Infrastructure' },
  { id: 'docker-services', label: 'Containers', description: 'Running containers', href: '/docker-services', icon: 'docker', category: 'Infrastructure' },
  { id: 'helm-repositories', label: 'Helm Repos', description: 'Helm chart repositories', href: '/helm-repositories', icon: 'helm', category: 'Infrastructure' },
  { id: 'workflows', label: 'Workflows', description: 'Workflow automation board', href: '/workflows', icon: 'fluxcd', category: 'Automation' },
  { id: 'automation', label: 'Automation Tasks', description: 'Automated tasks and executions', href: '/automation', icon: 'ai', category: 'Automation' },
  { id: 'jira', label: 'Jira', description: 'Jira integration', href: '/jira', icon: 'jira', category: 'Automation' },
  { id: 'connections', label: 'Connections', description: 'External integrations', href: '/connections', icon: 'plugin', category: 'Integrations' },
  { id: 'plugins', label: 'Plugins', description: 'Installed plugins', href: '/plugins', icon: 'plugin', category: 'Integrations' },
  { id: 'marketplace', label: 'Marketplace', description: 'Plugin marketplace', href: '/marketplace', icon: 'marketplace', category: 'Integrations' },
  { id: 'settings', label: 'Settings', description: 'Platform settings', href: '/settings', icon: 'plugin', category: 'Administration' },
  { id: 'roles', label: 'Roles', description: 'RBAC roles', href: '/roles', icon: 'plugin', category: 'Administration' },
  { id: 'vault', label: 'Vault', description: 'Encrypted secrets management', href: '/vault', icon: 'vault', category: 'Administration' },
  { id: 'audit', label: 'Audit Log', description: 'Activity history', href: '/audit', icon: 'discovery', category: 'Administration' },
  { id: 'plugin-activity', label: 'Plugin Activity', description: 'SSH commands & VM operations', href: '/plugin-activity', icon: 'plugin', category: 'Administration' },
  { id: 'security', label: 'Security', description: 'Security dashboard', href: '/security', icon: 'vault', category: 'Administration' },
];

// Quick actions
const actions: PaletteItem[] = [
  { id: 'new-service', label: 'Create New Service', description: 'Register a new service', href: '/services/new', icon: 'argocd', category: 'Actions' },
  { id: 'quick-deploy', label: 'Quick Deploy', description: 'Deploy a service wizard', href: '/deploy', icon: 'argocd', category: 'Actions' },
  { id: 'new-deploy', label: 'Trigger Deployment', description: 'GitOps deployment', href: '/deployments', icon: 'argocd', category: 'Actions' },
  { id: 'new-connection', label: 'Add Connection', description: 'Connect external tool', href: '/connections', icon: 'plugin', category: 'Actions' },
  { id: 'run-scan', label: 'Run Security Scan', description: 'Scan for vulnerabilities', href: '/security', icon: 'vault', category: 'Actions' },
  { id: 'ask-ai', label: 'Ask AI Assistant', description: 'Get AI help', href: '/ai', icon: 'ai', category: 'Actions' },
  { id: 'import-service', label: 'Import Service', description: 'Import from GitLab/GitHub', href: '/import', icon: 'gitlab', category: 'Actions' },
];

const RECENT_KEY = 'pepa-command-recent';
const MAX_RECENT = 5;

function getRecent(): string[] {
  if (typeof window === 'undefined') return [];
  try {
    return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]');
  } catch { return []; }
}

function addRecent(id: string) {
  const recent = getRecent().filter(r => r !== id);
  recent.unshift(id);
  localStorage.setItem(RECENT_KEY, JSON.stringify(recent.slice(0, MAX_RECENT)));
}

function fuzzyMatch(query: string, text: string): boolean {
  const q = query.toLowerCase();
  const t = text.toLowerCase();
  if (t.includes(q)) return true;
  let qi = 0;
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) qi++;
  }
  return qi === q.length;
}

// ── Dynamic entity fetching ────────────────────────────────────

interface EntitySource {
  key: string;
  category: string;
  icon: string;
  endpoint: string;
  mapFn: (data: Record<string, unknown>[]) => PaletteItem[];
}

// Lightweight fetch that uses the httpOnly cookie auth (same origin)
async function apiFetch<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(`${getBase()}${path}`);
    if (!res.ok) return null;
    return res.json() as Promise<T>;
  } catch {
    return null;
  }
}

const entitySources: EntitySource[] = [
  {
    key: 'service',
    category: 'Services',
    icon: 'storage',
    endpoint: '/api/v1/services?per_page=50&page=1',
    mapFn: (items) => items.map(s => ({
      id: `svc-${s.id}`,
      label: s.name as string,
      description: [s.language, s.framework].filter(Boolean).join(' · ') || (s.description as string) || 'Service',
      href: `/services/${s.id}`,
      icon: 'storage',
      category: 'Services',
    })),
  },
  {
    key: 'deployment',
    category: 'Deployments',
    icon: 'argocd',
    endpoint: '/api/v1/deployments',
    mapFn: (items) => items.map(d => ({
      id: `dpl-${d.id}`,
      label: (d.jira_summary || d.target_namespace || d.id) as string,
      description: [d.jira_issue_key, d.target_cluster_id, d.status].filter(Boolean).join(' · '),
      href: `/deployments/${d.id}`,
      icon: 'argocd',
      category: 'Deployments',
    })),
  },
  {
    key: 'cluster',
    category: 'Clusters',
    icon: 'kubernetes',
    endpoint: '/api/v1/clusters',
    mapFn: (items) => items.map(c => ({
      id: `cls-${c.id}`,
      label: c.name as string,
      description: [c.kubernetes_version, c.environment, `${c.node_count} nodes`].filter(Boolean).join(' · '),
      href: `/clusters/${c.id}`,
      icon: 'kubernetes',
      category: 'Clusters',
    })),
  },
  {
    key: 'environment',
    category: 'Environments',
    icon: 'discovery',
    endpoint: '/api/v1/environments',
    mapFn: (items) => items.map(e => ({
      id: `env-${e.id}`,
      label: e.name as string,
      description: [e.type, e.cluster, e.namespace, e.status].filter(Boolean).join(' · '),
      href: `/environments/${e.id}`,
      icon: 'discovery',
      category: 'Environments',
    })),
  },
  {
    key: 'connection',
    category: 'Connections',
    icon: 'plugin',
    endpoint: '/api/v1/connections',
    mapFn: (items) => items.map(c => ({
      id: `con-${c.id}`,
      label: c.name as string,
      description: [c.type, c.status].filter(Boolean).join(' · '),
      href: `/connections/${c.id}`,
      icon: (c.type as string)?.toLowerCase() || 'plugin',
      category: 'Connections',
    })),
  },
  {
    key: 'workflow',
    category: 'Workflows',
    icon: 'fluxcd',
    endpoint: '/api/v1/workflows',
    mapFn: (items) => items.map(w => ({
      id: `wf-${w.id}`,
      label: w.name as string,
      description: [w.source, w.is_enabled ? 'Enabled' : 'Disabled'].filter(Boolean).join(' · '),
      href: `/workflows/${w.id}`,
      icon: 'fluxcd',
      category: 'Workflows',
    })),
  },
  {
    key: 'docker-service',
    category: 'Containers',
    icon: 'docker',
    endpoint: '/api/v1/docker-services',
    mapFn: (items) => items.map(d => ({
      id: `dks-${d.id}`,
      label: d.name as string,
      description: [d.status, d.docker_host_id].filter(Boolean).join(' · '),
      href: `/docker-services/${d.id}`,
      icon: 'docker',
      category: 'Containers',
    })),
  },
  {
    key: 'helm-repo',
    category: 'Helm Repos',
    icon: 'helm',
    endpoint: '/api/v1/helm-repositories',
    mapFn: (items) => items.map(h => ({
      id: `hlm-${h.id}`,
      label: h.name as string,
      description: [h.repo_type, h.url, h.status].filter(Boolean).join(' · '),
      href: `/helm-repositories/${h.id}`,
      icon: 'helm',
      category: 'Helm Repos',
    })),
  },
  {
    key: 'pipeline-source',
    category: 'Pipeline Sources',
    icon: 'cicd',
    endpoint: '/api/v1/pipeline-sources',
    mapFn: (items) => items.map(p => ({
      id: `ps-${p.id}`,
      label: p.name as string,
      description: [p.source_type, p.description].filter(Boolean).join(' · '),
      href: `/pipelines`,
      icon: 'cicd',
      category: 'Pipeline Sources',
    })),
  },
];

export default function CommandPalette() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const router = useRouter();

  // Dynamic entity results fetched from the API
  const [dynamicItems, setDynamicItems] = useState<PaletteItem[]>([]);
  const [dynamicLoading, setDynamicLoading] = useState(false);
  // Track whether we've fetched at least once (cache until next open)
  const fetchedRef = useRef(false);

  // Fetch all entity sources in parallel when palette first opens
  const fetchEntities = useCallback(async () => {
    setDynamicLoading(true);
    const results = await Promise.all(
      entitySources.map(async (src) => {
        const data = await apiFetch<Record<string, unknown>>(src.endpoint);
        if (!data) return [];
        // Each endpoint returns a different wrapper key — find the array
        const arr = Object.values(data).find(v => Array.isArray(v)) as Record<string, unknown>[] | undefined;
        if (!arr) return [];
        return src.mapFn(arr);
      })
    );
    setDynamicItems(results.flat());
    setDynamicLoading(false);
    fetchedRef.current = true;
  }, []);

  // Toggle palette
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setOpen(prev => !prev);
      }
      if (e.key === 'Escape' && open) {
        setOpen(false);
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open]);

  // Focus input + fetch entities when opened
  useEffect(() => {
    if (open) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
      // Fetch entities on first open; subsequent opens use cached data
      if (!fetchedRef.current) {
        fetchEntities();
      }
    }
  }, [open, fetchEntities]);

  // Filter items — combines static pages/actions with dynamic entity results
  const filteredItems = useCallback(() => {
    const allItems = [...pages, ...actions, ...dynamicItems];
    if (!query.trim()) {
      // Show recent first, then pages (not dynamic items when query is empty)
      const recent = getRecent();
      const staticItems = [...pages, ...actions];
      const recentItems = recent.map(id => staticItems.find(i => i.id === id)).filter(Boolean) as PaletteItem[];
      const rest = staticItems.filter(i => !recent.includes(i.id));
      return [...recentItems, ...rest];
    }
    return allItems.filter(item =>
      fuzzyMatch(query, item.label) ||
      fuzzyMatch(query, item.description || '') ||
      fuzzyMatch(query, item.category)
    );
  }, [query, dynamicItems]);

  const items = filteredItems();

  // Keyboard navigation
  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIndex(prev => Math.min(prev + 1, items.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIndex(prev => Math.max(prev - 1, 0));
    } else if (e.key === 'Enter') {
      e.preventDefault();
      const item = items[selectedIndex];
      if (item) selectItem(item);
    } else if (e.key === 'Escape') {
      setOpen(false);
    }
  };

  const selectItem = (item: PaletteItem) => {
    addRecent(item.id);
    setOpen(false);
    if (item.href) {
      router.push(item.href);
    } else if (item.action) {
      item.action();
    }
  };

  // Scroll selected item into view
  useEffect(() => {
    if (listRef.current) {
      const selected = listRef.current.children[selectedIndex] as HTMLElement;
      if (selected) selected.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex]);

  if (!open) return null;

  // Group items by category
  const grouped: Record<string, PaletteItem[]> = {};
  for (const item of items) {
    if (!grouped[item.category]) grouped[item.category] = [];
    grouped[item.category].push(item);
  }

  let currentIndex = 0;

  return (
    <div className="fixed inset-0 z-[9999] flex items-start justify-center pt-[15vh]" onClick={() => setOpen(false)}>
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" />

      {/* Palette */}
      <div
        className="relative w-full max-w-[560px] bg-[var(--surface)] rounded-xl shadow-2xl border border-[var(--border)] overflow-hidden glass-palette"
        onClick={e => e.stopPropagation()}
      >
        {/* Search input */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-[var(--border-light)]">
          <svg className="w-5 h-5 text-[var(--text-tertiary)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            ref={inputRef}
            type="text"
            placeholder="Search pages, services, deployments, clusters..."
            value={query}
            onChange={e => { setQuery(e.target.value); setSelectedIndex(0); }}
            onKeyDown={handleKeyDown}
            className="flex-1 bg-transparent text-[14px] text-[var(--text-primary)] placeholder-[var(--text-tertiary)] outline-none"
          />
          {dynamicLoading && (
            <div className="w-4 h-4 border-2 border-[var(--accent)] border-t-transparent rounded-full animate-spin shrink-0" />
          )}
          <kbd className="hidden sm:inline-block px-1.5 py-0.5 text-[10px] font-medium text-[var(--text-tertiary)] bg-[var(--bg)] rounded border border-[var(--border)]">
            ESC
          </kbd>
        </div>

        {/* Results */}
        <div ref={listRef} className="max-h-[400px] overflow-y-auto py-2">
          {items.length === 0 ? (
            <div className="px-4 py-8 text-center">
              <p className="text-[13px] text-[var(--text-tertiary)]">No results for &ldquo;{query}&rdquo;</p>
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Try a different search term</p>
            </div>
          ) : (
            Object.entries(grouped).map(([category, categoryItems]) => (
              <div key={category}>
                <div className="px-4 py-1.5">
                  <span className="text-[10px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider">{category}</span>
                </div>
                {categoryItems.map(item => {
                  const idx = currentIndex++;
                  const isSelected = idx === selectedIndex;
                  return (
                    <button
                      key={item.id}
                      onClick={() => selectItem(item)}
                      onMouseEnter={() => setSelectedIndex(idx)}
                      className={`w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors ${
                        isSelected ? 'bg-[var(--bg)]' : 'hover:bg-[var(--bg)]'
                      }`}
                    >
                      <span className="w-6 text-center shrink-0"><BrandIcon name={item.icon} size={16} /></span>
                      <div className="flex-1 min-w-0">
                        <p className={`text-[13px] truncate ${isSelected ? 'text-[var(--accent)] font-medium' : 'text-[var(--text-primary)]'}`}>
                          {item.label}
                        </p>
                        {item.description && (
                          <p className="text-[11px] text-[var(--text-tertiary)] truncate">{item.description}</p>
                        )}
                      </div>
                      {item.href && (
                        <span className="text-[10px] text-[var(--text-tertiary)] shrink-0">↵</span>
                      )}
                    </button>
                  );
                })}
              </div>
            ))
          )}
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-4 py-2 border-t border-[var(--border-light)] bg-[var(--bg)]">
          <div className="flex items-center gap-3 text-[10px] text-[var(--text-tertiary)]">
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-[var(--surface)] rounded border border-[var(--border)]">↑↓</kbd>
              navigate
            </span>
            <span className="flex items-center gap-1">
              <kbd className="px-1 py-0.5 bg-[var(--surface)] rounded border border-[var(--border)]">↵</kbd>
              select
            </span>
          </div>
          <span className="text-[10px] text-[var(--text-tertiary)]">
            {items.length} result{items.length !== 1 ? 's' : ''}
          </span>
        </div>
      </div>
    </div>
  );
}
