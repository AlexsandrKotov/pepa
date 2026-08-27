'use client';

import ConceptHelp from '@/components/ConceptHelp';

export default function SecurityPage() {
  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      <div className="page-animate">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Security</h1>
            <ConceptHelp term="security" />
          </div>
          <p className="page-subtitle-modern">
            Manage security policies, audit logs, and access control
          </p>
        </div>
      </div>

      <div className="card card-body text-center py-20 page-animate-up page-delay-1" style={{ borderRadius: '16px' }}>
        <div className="text-5xl mb-2 opacity-20">🔒</div>
        <h2 className="text-[16px] font-semibold text-[var(--text-primary)]">
          Coming Soon
        </h2>
        <p className="text-[13px] text-[var(--text-secondary)] max-w-md mx-auto">
          Security policy management, security audit logs, and access control settings
          are planned for an upcoming release. For now, use the{' '}
          <a href="/audit" className="text-[var(--accent)] hover:underline">Audit Log</a>{' '}
          and{' '}
          <a href="/roles" className="text-[var(--accent)] hover:underline">Roles</a>{' '}
          pages for access control and activity tracking.
        </p>
      </div>
      </div>
    </div>
  );
}
