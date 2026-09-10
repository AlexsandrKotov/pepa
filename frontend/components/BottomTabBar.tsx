'use client';

import { usePathname } from 'next/navigation';
import Link from 'next/link';
import { memo, useMemo } from 'react';

interface TabItem {
  href: string;
  label: string;
  icon: string; // SVG path d attribute
  matchPrefix: string; // URL prefix for active matching
}

const tabs: TabItem[] = [
  {
    href: '/',
    label: 'Dashboard',
    icon: 'M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6',
    matchPrefix: '/',
  },
  {
    href: '/deploy',
    label: 'Deploy',
    icon: 'M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12',
    matchPrefix: '/deploy',
  },
  {
    href: '/services',
    label: 'Services',
    icon: 'M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4',
    matchPrefix: '/services',
  },
  {
    href: '/ai',
    label: 'AI',
    icon: 'M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 00-2.455 2.456z',
    matchPrefix: '/ai',
  },
  {
    href: '/settings',
    label: 'Settings',
    icon: 'M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z M15 12a3 3 0 11-6 0 3 3 0 016 0z',
    matchPrefix: '/settings',
  },
];

function isActiveTab(pathname: string, tab: TabItem): boolean {
  if (tab.href === '/') {
    return pathname === '/';
  }
  return pathname === tab.matchPrefix || pathname.startsWith(tab.matchPrefix + '/');
}

function BottomTabBarInner() {
  const pathname = usePathname();

  const activeTab = useMemo(() => {
    return tabs.find(tab => isActiveTab(pathname, tab));
  }, [pathname]);

  return (
    <nav
      className="md:hidden fixed bottom-0 left-0 right-0 z-[100] border-t border-[var(--border)]"
      style={{
        background: 'rgba(255,255,255,0.82)',
        backdropFilter: 'blur(16px) saturate(1.5)',
        WebkitBackdropFilter: 'blur(16px) saturate(1.5)',
        paddingBottom: 'env(safe-area-inset-bottom, 0px)',
      }}
      aria-label="Mobile navigation"
    >
      {/* Dark mode override */}
      <style>{`
        [data-theme="dark"] nav[aria-label="Mobile navigation"] {
          background: rgba(34,38,46,0.85) !important;
          border-color: var(--border) !important;
        }
      `}</style>
      <div className="flex items-center justify-around h-12">
        {tabs.map((tab) => {
          const active = activeTab?.href === tab.href;
          return (
            <Link
              key={tab.href}
              href={tab.href}
              className={`
                flex flex-col items-center justify-center gap-0.5 flex-1 h-full
                transition-colors duration-150
                ${active ? 'text-[var(--accent)]' : 'text-[var(--text-tertiary)]'}
              `}
              style={{ transitionTimingFunction: 'var(--ease-ios)' }}
            >
              <svg
                className="w-5 h-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={active ? 2 : 1.5}
              >
                <path strokeLinecap="round" strokeLinejoin="round" d={tab.icon} />
              </svg>
              <span className={`text-[10px] leading-tight ${active ? 'font-semibold' : 'font-normal'}`}>
                {tab.label}
              </span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}

const BottomTabBar = memo(BottomTabBarInner);
BottomTabBar.displayName = 'BottomTabBar';

export default BottomTabBar;
