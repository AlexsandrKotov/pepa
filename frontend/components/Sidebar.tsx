'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useState, useEffect, useMemo, useCallback } from 'react';
import { platformSettings, plugins, connections as connectionsAPI } from '@/lib/api';
import { usePermission } from '@/hooks/usePermission';

// Main navigation — pinned items (daily golden path), always visible
// permission: null means always visible (e.g. Dashboard)
const mainNav = [
  { href: '/', label: 'Dashboard', icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6', permission: null },
  { href: '/services/new', label: 'Deploy', icon: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12', permission: null },
];
const allMainHrefs = mainNav.map(i => i.href);

// Plugin-dependent nav items — shown only when the corresponding plugin is enabled
const pluginNavItems: Record<string, { section: string; item: { href: string; label: string; permission: string | null; adminOnly?: boolean } }> = {
};

// Plugin-dependent collapsible sections — entire sections shown only when plugin is enabled
const pluginSections: Record<string, { title: string; icon: string; items: { href: string; label: string; permission: string | null; adminOnly?: boolean }[]; subGroups?: NavSubGroup[] }> = {
  s3: {
    title: 'Object Storage',
    icon: 'M3 6c0-1.657 4.03-3 9-3s9 1.343 9 3v2c0 1.657-4.03 3-9 3S3 9.657 3 8V6zm0 4c0 1.657 4.03 3 9 3s9-1.343 9-3v2c0 1.657-4.03 3-9 3s-9-1.343-9-3v-2zm0 4c0 1.657 4.03 3 9 3s9-1.343 9-3v2c0 1.657-4.03 3-9 3s-9-1.343-9-3v-2z',
    items: [
      { href: '/s3-manage', label: 'S3 Manage', permission: 'connections' },
    ],
  },
  jira: {
    title: 'Jira Integration',
    icon: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4',
    items: [
      { href: '/jira', label: 'Jira', permission: 'jira' },
    ],
  },
  'remote-console': {
    title: 'Remote Access',
    icon: 'M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z',
    items: [
      { href: '/remote-console', label: 'SSH Console', permission: null },
    ],
  },
};

// Collapsible sections for additional pages
// Sections can optionally use `subGroups` for visual grouping of items within the section.
interface NavSubGroup {
  label: string;
  items: { href: string; label: string; permission: string | null; adminOnly?: boolean }[];
}

// Connection-driven collapsible sections — shown when connections of the matching type exist.
// Each key maps to a connection type; sub-groups are filtered by which types have connections.
const connectionDrivenSections: Record<string, { title: string; icon: string; connectionTypes: string[]; subGroups: Record<string, NavSubGroup> }> = {
  virtualization: {
    title: 'Virtualization',
    icon: 'M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2',
    connectionTypes: ['proxmox', 'vmware'],
    subGroups: {
      proxmox: {
        label: 'Proxmox',
        items: [
          { href: '/virtualization', label: 'Dashboard', permission: 'virtualization' },
          { href: '/virtualization/hosts', label: 'Hosts', permission: 'virtualization' },
          { href: '/virtualization/vms', label: 'Virtual Machines', permission: 'virtualization' },
          { href: '/virtualization/containers', label: 'Containers', permission: 'virtualization' },
          { href: '/virtualization/logs', label: 'Logs', permission: 'virtualization' },
        ],
      },
      vmware: {
        label: 'VMware',
        items: [
          { href: '/virtualization', label: 'Dashboard', permission: 'virtualization' },
          { href: '/virtualization/hosts', label: 'Hosts', permission: 'virtualization' },
          { href: '/virtualization/vms', label: 'Virtual Machines', permission: 'virtualization' },
        ],
      },
    },
  },
};

// Grouped by user journey: Catalog → Delivery → Infrastructure → Automation → Integrations → Administration.
// Developer-facing sections come first, admin-facing last.
const collapsibleSections: ({ title: string; icon: string; adminOnly?: boolean; items: { href: string; label: string; permission: string | null; adminOnly?: boolean }[]; subGroups?: NavSubGroup[] })[] = [
  {
    title: 'Catalog',
    icon: 'M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4',
    items: [
      { href: '/services', label: 'Services', permission: null },
      { href: '/entities', label: 'Entities', permission: 'entities' },
      { href: '/scorecards', label: 'Scorecards', permission: 'scorecards' },
      { href: '/discovery', label: 'Discovery', permission: 'discovery' },
      { href: '/import', label: 'Import', permission: 'import' },
      { href: '/export', label: 'Export', permission: 'import' },
    ],
  },
  {
    title: 'Delivery',
    icon: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12',
    items: [],
    subGroups: [
      {
        label: 'Pipelines',
        items: [
          { href: '/pipelines', label: 'Runs', permission: 'pipelines' },
          { href: '/pipeline-builder', label: 'Builder', permission: 'pipelines' },
          { href: '/pipeline-blueprints', label: 'Blueprints', permission: 'pipelines' },
          { href: '/blueprint-groups', label: 'Blueprint Groups', permission: 'pipelines' },
        ],
      },
      {
        label: 'Releases',
        items: [
          { href: '/deployments', label: 'Deployments', permission: 'deployments' },
          { href: '/environments', label: 'Environments', permission: 'environments' },
        ],
      },
      {
        label: 'GitOps',
        items: [
          { href: '/gitops', label: 'Overview', permission: 'gitops' },
          { href: '/gitops/drift', label: 'Drift Detection', permission: 'gitops' },
        ],
      },
    ],
  },
  {
    title: 'Infrastructure',
    icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10',
    items: [
      { href: '/clusters', label: 'Clusters', permission: 'clusters', adminOnly: true },
      { href: '/docker-hosts', label: 'Docker Hosts', permission: 'docker', adminOnly: true },
      { href: '/docker-services', label: 'Containers', permission: 'docker' },
      { href: '/helm-repositories', label: 'Helm Repos', permission: 'helm', adminOnly: true },
    ],
  },
  {
    title: 'Vault',
    icon: 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
    items: [
      { href: '/vault', label: 'Secrets', permission: 'vault' },
    ],
  },
  {
    title: 'Delivery Automation',
    icon: 'M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664zM21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    items: [
      { href: '/automation', label: 'Automation', permission: 'workflows' },
      { href: '/workflows', label: 'GitOps Workflow', permission: 'workflows' },
      { href: '/pipelines/new', label: 'Pipeline Builder', permission: 'workflows' },
    ],
  },
  {
    title: 'Integrations',
    icon: 'M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1',
    adminOnly: true,
    items: [
      { href: '/connections', label: 'Connections', permission: 'connections' },
      { href: '/plugins', label: 'Plugins', permission: 'plugins' },
      { href: '/marketplace', label: 'Marketplace', permission: 'plugins' },
    ],
  },
  {
    title: 'Administration',
    icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
    adminOnly: true,
    items: [
      { href: '/settings', label: 'Settings', permission: 'settings', adminOnly: true },
      { href: '/settings/authentication', label: 'Authentication', permission: 'settings', adminOnly: true },
      { href: '/settings/users', label: 'Users', permission: 'settings', adminOnly: true },
      { href: '/settings/teams', label: 'Teams', permission: 'settings', adminOnly: true },
      { href: '/settings/workspaces', label: 'Workspaces', permission: 'settings', adminOnly: true },
      { href: '/settings/observability', label: 'Observability', permission: 'settings', adminOnly: true },
      { href: '/roles', label: 'Roles', permission: 'roles', adminOnly: true },
      { href: '/audit', label: 'Audit Log', permission: 'audit', adminOnly: true },
      { href: '/plugin-activity', label: 'Plugin Activity', permission: 'plugin_activity', adminOnly: true },
    ],
  },
];

// Every href across all sections — lets isPathActive suppress shorter
// prefix matches (e.g., '/pipelines' must not highlight on '/pipelines/new').
// '/pipelines/edit' is not a nav item but belongs to the Builder, so it is
// listed here to keep 'Runs' inactive while editing a pipeline.
const allNavHrefs = [
  ...allMainHrefs,
  '/credentials',
  '/ai',
  '/knowledge-base',
  '/get-started',
  ...collapsibleSections.flatMap(s =>
    s.subGroups ? s.subGroups.flatMap(sg => sg.items.map(i => i.href)) : s.items.map(i => i.href)
  ),
  ...Object.values(pluginNavItems).map(e => e.item.href),
  ...Object.values(pluginSections).flatMap(s =>
    s.subGroups ? s.subGroups.flatMap(sg => sg.items.map(i => i.href)) : s.items.map(i => i.href)
  ),
  ...Object.values(connectionDrivenSections).flatMap(s =>
    Object.values(s.subGroups).flatMap(sg => sg.items.map(i => i.href))
  ),
  '/pipelines/edit',
];

/** Exact path match — avoids prefix false positives.
 *  When allHrefs is provided, only the longest matching href wins.
 *  Query parameters in href are stripped before comparison. */
function isPathActive(pathname: string, href: string, allHrefs?: string[]): boolean {
  // Strip query string and hash from href for path comparison
  const cleanHref = href.split('?')[0].split('#')[0];
  if (cleanHref === '/') return pathname === '/';
  const matches = pathname === cleanHref || pathname.startsWith(cleanHref + '/');
  if (!matches) return false;
  if (allHrefs) {
    for (const other of allHrefs) {
      const cleanOther = other.split('?')[0].split('#')[0];
      if (cleanOther === cleanHref || cleanOther === '/') continue;
      if ((pathname === cleanOther || pathname.startsWith(cleanOther + '/')) && cleanOther.length > cleanHref.length) {
        return false;
      }
    }
  }
  return true;
}

function NavItem({ item, active, collapsed }: { item: { href: string; label: string; icon: string }; active: boolean; collapsed: boolean }) {
  return (
    <Link
      href={item.href}
      prefetch={true}
      className={`group flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] transition-all duration-150 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
        active
          ? 'bg-white/10 text-white shadow-sm'
          : 'text-white/40 hover:text-white/80 hover:bg-white/[0.04]'
      }`}
      title={collapsed ? item.label : undefined}
    >
      <svg className={`w-[17px] h-[17px] shrink-0 ${active ? 'text-white' : 'text-white/30 group-hover:text-white/60'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d={item.icon} />
      </svg>
      {!collapsed && <span className="truncate">{item.label}</span>}
    </Link>
  );
}

function CollapsibleSection({ section, collapsed, expandedSections, toggleSection, pathname }: {
  section: typeof collapsibleSections[0];
  collapsed: boolean;
  expandedSections: Set<string>;
  toggleSection: (title: string) => void;
  pathname: string;
}) {
  const isExpanded = expandedSections.has(section.title);
  // Merge items + subGroup items for active-check and rendering
  const allItems = section.subGroups
    ? section.subGroups.flatMap(sg => sg.items)
    : section.items;
  const allHrefs = allNavHrefs;
  const hasActiveChild = allItems.some(item => isPathActive(pathname, item.href, allHrefs));

  return (
    <div className="space-y-0.5">
      <button
        onClick={() => toggleSection(section.title)}
        className={`group flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] transition-all duration-150 w-full outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
          hasActiveChild
            ? 'text-white/70'
            : 'text-white/40 hover:text-white/80 hover:bg-white/[0.04]'
        }`}
        title={collapsed ? section.title : undefined}
      >
        <svg className={`w-[17px] h-[17px] shrink-0 ${hasActiveChild ? 'text-white/50' : 'text-white/30 group-hover:text-white/60'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d={section.icon} />
        </svg>
        {!collapsed && (
          <>
            <span className="truncate flex-1 text-left">{section.title}</span>
            <svg className={`w-3 h-3 transition-transform ${isExpanded ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
            </svg>
          </>
        )}
      </button>
      {!collapsed && isExpanded && (
        <div className="ml-4 space-y-0.5">
          {section.subGroups ? (
            <>
              {section.subGroups.map((sg, sgIdx) => (
                <div key={sg.label} className={sgIdx > 0 ? 'mt-3 pt-3 border-t border-white/[0.06]' : ''}>
                  <div className="px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider text-white/30">
                    {sg.label}
                  </div>
                  {sg.items.map((item) => {
                    const itemActive = isPathActive(pathname, item.href, allHrefs);
                    return (
                      <Link
                        key={item.href}
                        href={item.href}
                        className={`flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-[12px] transition-all duration-150 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
                          itemActive
                            ? 'bg-white/10 text-white font-medium'
                            : 'text-white/30 hover:text-white/70 hover:bg-white/[0.04]'
                        }`}
                      >
                        {itemActive && <span className="w-1 h-1 rounded-full bg-[var(--accent)] shrink-0" />}
                        <span className="truncate">{item.label}</span>
                      </Link>
                    );
                  })}
                </div>
              ))}
            </>
          ) : (
            section.items.map((item) => {
              const itemActive = isPathActive(pathname, item.href, allHrefs);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={`flex items-center gap-2 px-2.5 py-1.5 rounded-lg text-[12px] transition-all duration-150 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
                    itemActive
                      ? 'bg-white/10 text-white font-medium'
                      : 'text-white/30 hover:text-white/70 hover:bg-white/[0.04]'
                  }`}
                >
                  {itemActive && <span className="w-1 h-1 rounded-full bg-[var(--accent)] shrink-0" />}
                  <span className="truncate">{item.label}</span>
                </Link>
              );
            })
          )}
        </div>
      )}
    </div>
  );
}

export default function Sidebar() {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(false);
  const [expandedSections, setExpandedSections] = useState<Set<string>>(new Set());
  // 'unknown' until fetched — avoids hiding Get Started before the check completes
  const [tourStatus, setTourStatus] = useState<'unknown' | 'incomplete' | 'done'>('unknown');
  const [enabledPlugins, setEnabledPlugins] = useState<Set<string>>(new Set());
  const [connectionTypes, setConnectionTypes] = useState<Set<string>>(new Set());
  const { hasPermission, isAdmin } = usePermission();

  // Helper: check if user can see a nav item (permission: null = always visible)
  // adminOnly items require admin role OR explicit write permission (not just read)
  const canSee = (perm: string | null, adminOnly?: boolean) => {
    if (adminOnly) {
      // Admin-only items: require admin role OR write permission
      return isAdmin || (perm ? hasPermission(perm, 'create') || hasPermission(perm, 'update') || hasPermission(perm, 'delete') : false);
    }
    return !perm || hasPermission(perm, 'read');
  };

  // Filter mainNav items based on permissions
  const visibleMainNav = useMemo(
    () => mainNav.filter(item => canSee(item.permission)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [hasPermission],
  );

  // Fetch plugin states to conditionally show plugin tabs
  useEffect(() => {
    plugins.list().then(res => {
      const enabled = new Set<string>();
      for (const p of res.plugins || []) {
        if (p.enabled) enabled.add(p.name);
      }
      setEnabledPlugins(enabled);
    }).catch(() => {});
  }, []);

  // Fetch connection types to conditionally show connection-driven sections (e.g. Virtualization)
  useEffect(() => {
    connectionsAPI.list().then(res => {
      const types = new Set<string>();
      for (const c of res.connections || []) {
        types.add(c.type);
      }
      setConnectionTypes(types);
    }).catch(() => {});
  }, []);

  // Get Started hides itself once the guided tour is completed.
  // Deferred: only checked after initial paint to avoid blocking sidebar render
  const checkTourStatus = useCallback(() => {
    platformSettings.get('get_started')
      .then(res => {
        const v = res.value as { completed?: boolean } | undefined;
        setTourStatus(v?.completed ? 'done' : 'incomplete');
      })
      .catch(() => setTourStatus('incomplete'));
  }, []);

  useEffect(() => {
    const timer = setTimeout(checkTourStatus, 1500);
    // Listen for custom event dispatched from the get-started page
    window.addEventListener('pepa-tour-completed', checkTourStatus);
    return () => {
      clearTimeout(timer);
      window.removeEventListener('pepa-tour-completed', checkTourStatus);
    };
  }, [checkTourStatus]);

  // Re-check on navigation — hides the tab as soon as the user leaves the get-started page
  useEffect(() => {
    if (tourStatus !== 'done') checkTourStatus();
  }, [pathname]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const saved = localStorage.getItem('pepa-sidebar');
    if (saved === 'collapsed') setCollapsed(true);
    
    const savedSections = localStorage.getItem('pepa-sidebar-sections');
    if (savedSections) {
      try {
        setExpandedSections(new Set(JSON.parse(savedSections)));
      } catch (e) {
        // Ignore parse errors
      }
    }
  }, []);

  // Auto-expand the section that contains the current page
  useEffect(() => {
    for (const section of collapsibleSections) {
      const allItems = section.subGroups
        ? section.subGroups.flatMap(sg => sg.items)
        : section.items;
      // allNavHrefs enables longest-match suppression: '/pipelines/new' must
      // not auto-expand Delivery via its '/pipelines' prefix.
      if (allItems.some(item => isPathActive(pathname, item.href, allNavHrefs))) {
        if (!expandedSections.has(section.title)) {
          const next = new Set(expandedSections);
          next.add(section.title);
          setExpandedSections(next);
          localStorage.setItem('pepa-sidebar-sections', JSON.stringify(Array.from(next)));
        }
        break;
      }
    }
  }, [pathname, enabledPlugins, connectionTypes]); // eslint-disable-line react-hooks/exhaustive-deps

  const toggle = () => {
    setCollapsed(!collapsed);
    localStorage.setItem('pepa-sidebar', !collapsed ? 'collapsed' : 'expanded');
  };

  const toggleSection = (title: string) => {
    const next = new Set(expandedSections);
    if (next.has(title)) next.delete(title);
    else next.add(title);
    setExpandedSections(next);
    localStorage.setItem('pepa-sidebar-sections', JSON.stringify(Array.from(next)));
  };

  // Build dynamic sections with plugin-dependent items, filtered by permissions
  const dynamicSections = useMemo(() => {
    const sections = collapsibleSections
      .map(section => {
        // Section-level adminOnly: hide entire section unless admin or has any item permission
        if (section.adminOnly && !isAdmin) {
          const hasAnyItemPerm = section.items.some(i => i.permission && hasPermission(i.permission, 'read'));
          if (!hasAnyItemPerm) return null;
        }

        const extraItems: { href: string; label: string; permission: string | null; adminOnly?: boolean }[] = [];
        for (const [pluginName, navEntry] of Object.entries(pluginNavItems)) {
          if (navEntry.section === section.title && enabledPlugins.has(pluginName)) {
            extraItems.push(navEntry.item);
          }
        }
        let updated = section;
        if (extraItems.length > 0) {
          if (section.subGroups && section.subGroups.length > 0) {
            const updatedSubGroups = section.subGroups.map((sg, idx) =>
              idx === section.subGroups!.length - 1
                ? { ...sg, items: [...sg.items, ...extraItems] }
                : sg
            );
            updated = { ...section, subGroups: updatedSubGroups };
          } else {
            updated = { ...section, items: [...section.items, ...extraItems] };
          }
        }

        // Filter items by permission (adminOnly items visible only to admins)
        if (updated.subGroups) {
          const filteredSubGroups = updated.subGroups
            .map(sg => ({ ...sg, items: sg.items.filter(i => canSee(i.permission, i.adminOnly)) }))
            .filter(sg => sg.items.length > 0);
          if (filteredSubGroups.length === 0 && updated.items.filter(i => canSee(i.permission, i.adminOnly)).length === 0) {
            return null; // entire section hidden
          }
          return { ...updated, subGroups: filteredSubGroups, items: updated.items.filter(i => canSee(i.permission, i.adminOnly)) };
        }
        const filteredItems = updated.items.filter(i => canSee(i.permission, i.adminOnly));
        if (filteredItems.length === 0) return null; // entire section hidden
        return { ...updated, items: filteredItems };
      })
      .filter(Boolean) as typeof collapsibleSections;

    // Add plugin-dependent sections (e.g., Object Storage for s3, Jira)
    const extraSections: typeof collapsibleSections = [];
    for (const [pluginName, section] of Object.entries(pluginSections)) {
      if (enabledPlugins.has(pluginName)) {
        // Filter items by permission
        if (section.subGroups) {
          const filteredSubGroups = section.subGroups
            .map(sg => ({ ...sg, items: sg.items.filter(i => canSee(i.permission, i.adminOnly)) }))
            .filter(sg => sg.items.length > 0);
          if (filteredSubGroups.length > 0) {
            extraSections.push({ ...section, subGroups: filteredSubGroups });
          }
        } else {
          const filteredItems = section.items.filter(i => canSee(i.permission, i.adminOnly));
          if (filteredItems.length > 0) {
            extraSections.push({ ...section, items: filteredItems });
          }
        }
      }
    }

    // Add connection-driven sections (e.g., Virtualization when proxmox/vmware connections exist)
    for (const [, cfg] of Object.entries(connectionDrivenSections)) {
      const activeSubGroups: NavSubGroup[] = [];
      for (const connType of cfg.connectionTypes) {
        if (connectionTypes.has(connType) && cfg.subGroups[connType]) {
          const sg = cfg.subGroups[connType];
          const filtered = sg.items.filter(i => canSee(i.permission, i.adminOnly));
          if (filtered.length > 0) {
            activeSubGroups.push({ ...sg, items: filtered });
          }
        }
      }
      if (activeSubGroups.length > 0) {
        extraSections.push({
          title: cfg.title,
          icon: cfg.icon,
          items: [],
          subGroups: activeSubGroups,
        });
      }
    }

    // Insert extra sections after Infrastructure
    const infraIdx = sections.findIndex(s => s.title === 'Infrastructure');
    if (infraIdx >= 0 && extraSections.length > 0) {
      sections.splice(infraIdx + 1, 0, ...extraSections);
    } else {
      sections.push(...extraSections);
    }

    return sections;
  }, [enabledPlugins, connectionTypes, hasPermission, isAdmin]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <aside className={`flex flex-col h-full bg-[#1e2128] text-white transition-all duration-300 ease-in-out ${collapsed ? 'w-[56px]' : 'w-[190px]'}`}>
      {/* Logo */}
      <div className="flex items-center h-[52px] px-3.5 shrink-0">
        <div className="flex items-center gap-2.5 min-w-0">
          <div className="w-7 h-7 shrink-0">
            <svg viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
              <defs>
                <linearGradient id="sb-bg" x1="0" y1="0" x2="64" y2="64" gradientUnits="userSpaceOnUse">
                  <stop offset="0%" stopColor="#0052CC"/>
                  <stop offset="100%" stopColor="#6E56CF"/>
                </linearGradient>
              </defs>
              <rect x="2" y="2" width="60" height="60" rx="14" fill="url(#sb-bg)"/>
              <rect x="2" y="2" width="60" height="30" rx="14" fill="#fff" opacity="0.06"/>
              <path d="M20 46V18h14a10 10 0 010 20H20" stroke="#fff" strokeOpacity="0.9" strokeWidth="4.5" strokeLinecap="round" strokeLinejoin="round" fill="none"/>
              <circle cx="44" cy="42" r="3.5" fill="#fff" fillOpacity="0.2"/>
              <circle cx="54" cy="34" r="3" fill="#fff" fillOpacity="0.15"/>
              <circle cx="54" cy="50" r="3" fill="#fff" fillOpacity="0.15"/>
              <line x1="47" y1="41" x2="51.5" y2="35.5" stroke="#fff" strokeWidth="1" strokeLinecap="round" opacity="0.25"/>
              <line x1="47" y1="43" x2="51.5" y2="48.5" stroke="#fff" strokeWidth="1" strokeLinecap="round" opacity="0.25"/>
            </svg>
          </div>
          {!collapsed && (
            <div className="flex flex-col">
              <span className="font-semibold text-sm leading-tight">PEPA</span>
              <span className="text-[9px] text-white/30 leading-tight">Platform Engine</span>
            </div>
          )}
        </div>
      </div>

      {/* User Quick Access */}
      {!collapsed && (
        <div className="px-2 pb-2 shrink-0">
          <Link
            href="/credentials"
            className={`group flex items-center gap-2.5 px-2.5 py-1.5 rounded-lg text-[12px] transition-all duration-150 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
              isPathActive(pathname, '/credentials', allNavHrefs)
                ? 'bg-white/10 text-white'
                : 'text-white/40 hover:text-white/70 hover:bg-white/[0.04]'
            }`}
          >
            <svg className={`w-4 h-4 shrink-0 ${isPathActive(pathname, '/credentials') ? 'text-white' : 'text-white/30 group-hover:text-white/60'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
            </svg>
            <span className="truncate">My Credentials</span>
          </Link>
        </div>
      )}

      {/* Main Nav */}
      <nav className="flex-1 px-2 pt-2 space-y-0.5 overflow-y-auto">
        {/* AI Platform — flagship feature, pinned to the very top */}
        {canSee('ai') && (
          <Link
            href="/ai"
            prefetch={true}
            className={`group flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] transition-all duration-150 mb-1 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
              isPathActive(pathname, '/ai')
                ? 'bg-white/10 text-white shadow-sm'
                : 'text-white/40 hover:text-white/80 hover:bg-white/[0.04]'
            }`}
            title={collapsed ? 'AI Platform' : undefined}
          >
            <svg className={`w-[17px] h-[17px] shrink-0 ${isPathActive(pathname, '/ai') ? 'text-white' : 'text-white/30 group-hover:text-white/60'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.456 2.456L21.75 6l-1.035.259a3.375 3.375 0 00-2.456 2.456z" />
            </svg>
            {!collapsed && <span className="truncate">AI Platform</span>}
          </Link>
        )}

        {/* Knowledge Base — RAG-powered AI context */}
        {canSee('ai') && (
          <Link
            href="/knowledge-base"
            prefetch={true}
            className={`group flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] transition-all duration-150 mb-1 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
              isPathActive(pathname, '/knowledge-base')
                ? 'bg-white/10 text-white shadow-sm'
                : 'text-white/40 hover:text-white/80 hover:bg-white/[0.04]'
            }`}
            title={collapsed ? 'Knowledge Base' : undefined}
          >
            <svg className={`w-[17px] h-[17px] shrink-0 ${isPathActive(pathname, '/knowledge-base') ? 'text-white' : 'text-white/30 group-hover:text-white/60'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 6.042A8.967 8.967 0 006 3.75c-1.052 0-2.062.18-3 .512v14.25A8.987 8.987 0 016 18c2.305 0 4.408.867 6 2.292m0-14.25a8.966 8.966 0 016-2.292c1.052 0 2.062.18 3 .512v14.25A8.987 8.987 0 0018 18a8.967 8.967 0 00-6 2.292m0-14.25v14.25" />
            </svg>
            {!collapsed && <span className="truncate">Knowledge Base</span>}
          </Link>
        )}

        {/* Get Started — guided tour for newcomers, hidden once completed */}
        {tourStatus !== 'done' && (
        <Link
          href="/get-started"
          className={`group flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[13px] transition-all duration-150 mb-1 outline-none focus-visible:ring-1 focus-visible:ring-white/20 ${
            isPathActive(pathname, '/get-started')
              ? 'bg-white/10 text-white shadow-sm'
              : 'text-white/40 hover:text-white/80 hover:bg-white/[0.04]'
          }`}
          title={collapsed ? 'Get Started' : undefined}
        >
          <svg className={`w-[17px] h-[17px] shrink-0 ${isPathActive(pathname, '/get-started') ? 'text-white' : 'text-white/30 group-hover:text-white/60'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          {!collapsed && (
            <>
              <span className="truncate flex-1">Get Started</span>
              {tourStatus === 'incomplete' && <span className="w-1.5 h-1.5 rounded-full bg-[var(--accent)] shrink-0" />}
            </>
          )}
        </Link>
        )}

        {visibleMainNav.map((item) => (
          <NavItem key={item.href} item={item} active={isPathActive(pathname, item.href, allNavHrefs)} collapsed={collapsed} />
        ))}
        
        {/* Collapsible Sections */}
        {!collapsed && (
          <div className="mt-4 pt-4 border-t border-white/[0.06] space-y-0.5">
            {dynamicSections.map((section) => (
              <CollapsibleSection
                key={section.title}
                section={section}
                collapsed={collapsed}
                expandedSections={expandedSections}
                toggleSection={toggleSection}
                pathname={pathname}
              />
            ))}
          </div>
        )}
      </nav>

      {/* Footer */}
      <div className="px-2 pb-3 shrink-0">
        <div className="border-t border-white/[0.06] pt-2 space-y-0.5">
          <button
            onClick={toggle}
            className="flex items-center gap-2.5 px-2.5 py-2 rounded-lg text-[12px] text-white/20 hover:text-white/50 hover:bg-white/[0.03] w-full transition-colors"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              {collapsed ? (
                <path strokeLinecap="round" strokeLinejoin="round" d="M13 5l7 7-7 7M5 5l7 7-7 7" />
              ) : (
                <path strokeLinecap="round" strokeLinejoin="round" d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
              )}
            </svg>
            {!collapsed && <span>Collapse</span>}
          </button>
        </div>
      </div>
    </aside>
  );
}
