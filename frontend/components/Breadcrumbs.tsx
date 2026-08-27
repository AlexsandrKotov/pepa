'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';

// Map path segments to human-readable names
const segmentNames: Record<string, string> = {
  'services': 'Services',
  'connections': 'Connections',
  'deployments': 'Deployments',
  'clusters': 'Clusters',
  'workflows': 'Workflows',
  'automation': 'Automation',
  'environments': 'Environments',
  'import': 'Import',
  'vault': 'Vault',
  'jira': 'Jira',
  'plugins': 'Plugins',
  'policies': 'Policies',
  'security': 'Security',
  'integrations': 'Integrations',
  'observability': 'Observability',
  'optimization': 'Optimization',
  'analytics': 'Analytics',
  'compliance': 'Compliance',
  'scorecards': 'Scorecards',
  'ai': 'AI Assistant',
  'graph': 'Graph',
  'audit': 'Audit Log',
  'roles': 'Roles',
  'templates': 'Templates',
  'docs': 'Documentation',
  'marketplace': 'Marketplace',
  'settings': 'Settings',
  'new': 'New',
  'edit': 'Edit',
  'run': 'Run',
  'runs': 'Runs',
  'blueprints': 'Blueprints',
  'designer': 'Designer',
};

// "docker-hosts" -> "Docker Hosts"
function prettifySegment(seg: string): string {
  return seg
    .split('-')
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ');
}

export default function Breadcrumbs() {
  const pathname = usePathname();

  // Don't show on home page
  if (pathname === '/') return null;

  const segments = pathname.split('/').filter(Boolean);

  // Don't show if only one segment (top-level page)
  if (segments.length <= 1) return null;

  const crumbs = segments.map((seg, idx) => {
    const href = '/' + segments.slice(0, idx + 1).join('/');
    const isLast = idx === segments.length - 1;
    // Check if it's a UUID (detail page) — show truncated ID
    const isUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(seg);
    const label = isUUID
      ? seg.slice(0, 8) + '...'
      : segmentNames[seg] || prettifySegment(seg);

    return { href, label, isLast };
  });

  return (
    <nav className="flex items-center gap-1.5 text-[12px] text-[#a3a3a3] mb-4">
      <Link href="/" className="hover:text-[#525252] transition-colors">
        Dashboard
      </Link>
      {crumbs.map((crumb, idx) => (
        <span key={crumb.href} className="flex items-center gap-1.5">
          <svg className="w-3 h-3 text-[#d4d4d4]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          {crumb.isLast ? (
            <span className="text-[#525252] font-medium truncate max-w-[160px]">{crumb.label}</span>
          ) : (
            <Link href={crumb.href} className="hover:text-[#525252] transition-colors truncate max-w-[160px]">
              {crumb.label}
            </Link>
          )}
        </span>
      ))}
    </nav>
  );
}
