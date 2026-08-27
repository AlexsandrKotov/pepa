'use client';

import { useState, useRef, useEffect } from 'react';
import Link from 'next/link';

interface QuickAction {
  label: string;
  href: string;
  icon: string;
  description: string;
  color: string;
}

const quickActions: QuickAction[] = [
  { label: 'New Service', href: '/services/new', icon: '📦', description: 'Register a service', color: 'bg-blue-500/10 text-blue-500 hover:bg-blue-500/15' },
  { label: 'Deploy', href: '/deployments', icon: '🚀', description: 'Trigger deployment', color: 'bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/15' },
  { label: 'Connection', href: '/connections', icon: '🔗', description: 'Add integration', color: 'bg-violet-500/10 text-violet-500 hover:bg-violet-500/15' },
  { label: 'Security Scan', href: '/security', icon: '🛡️', description: 'Run vulnerability scan', color: 'bg-amber-500/10 text-amber-600 hover:bg-amber-500/15' },
  { label: 'AI Assistant', href: '/ai', icon: '🤖', description: 'Ask AI for help', color: 'bg-pink-500/10 text-pink-500 hover:bg-pink-500/15' },
  { label: 'Import', href: '/import', icon: '📥', description: 'Import from Git', color: 'bg-cyan-500/10 text-cyan-500 hover:bg-cyan-500/15' },
];

export default function QuickActions() {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // Close on outside click
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    if (isOpen) document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  // Close on Escape
  useEffect(() => {
    const handleEsc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setIsOpen(false);
    };
    document.addEventListener('keydown', handleEsc);
    return () => document.removeEventListener('keydown', handleEsc);
  }, []);

  return (
    <div ref={containerRef} className="fixed bottom-6 right-6 z-50">
      {/* Action menu */}
      {isOpen && (
        <div className="absolute bottom-14 right-0 w-56 bg-white rounded-xl shadow-xl border border-[#e5e5e5] overflow-hidden mb-2 animate-in fade-in slide-in-from-bottom-2 duration-150">
          <div className="px-3 py-2 border-b border-[#f0f0f0]">
            <p className="text-[10px] font-semibold text-[#a3a3a3] uppercase tracking-wider">Quick Actions</p>
          </div>
          <div className="py-1">
            {quickActions.map(action => (
              <Link
                key={action.label}
                href={action.href}
                onClick={() => setIsOpen(false)}
                className="flex items-center gap-3 px-3 py-2.5 hover:bg-[#fafafa] transition-colors group"
              >
                <div className={`w-8 h-8 rounded-lg flex items-center justify-center text-[14px] ${action.color} transition-colors`}>
                  {action.icon}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="text-[13px] font-medium text-[#171717] group-hover:text-[#0066ff] transition-colors">{action.label}</p>
                  <p className="text-[10px] text-[#a3a3a3]">{action.description}</p>
                </div>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* FAB button */}
      <button
        onClick={() => setIsOpen(!isOpen)}
        className={`w-12 h-12 rounded-full bg-[var(--accent)] text-white shadow-lg flex items-center justify-center transition-all duration-200 hover:opacity-90 hover:shadow-xl ${
          isOpen ? 'rotate-45' : ''
        }`}
        title="Quick Actions"
      >
        <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
        </svg>
      </button>
    </div>
  );
}
