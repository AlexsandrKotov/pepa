import Link from 'next/link';

export default function NotFound() {
  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate">
          <h1 className="page-title-modern">Page not found</h1>
          <p className="page-subtitle-modern">The page you&apos;re looking for doesn&apos;t exist or has been moved</p>
        </div>

        <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="card-body">
            <div className="flex items-start gap-4">
              <div className="w-10 h-10 rounded-full bg-[var(--border-light)] flex items-center justify-center shrink-0">
                <svg className="w-5 h-5 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-[13px] text-[var(--text-primary)] font-medium mb-1">404</p>
                <p className="text-[13px] text-[var(--text-secondary)]">
                  The page you&apos;re looking for doesn&apos;t exist or has been moved.
                </p>
              </div>
            </div>
          </div>
          <div className="card-footer flex items-center gap-3">
            <Link href="/" className="btn btn-primary btn-sm">
              Go to Dashboard
            </Link>
            <Link href="javascript:history.back()" className="btn btn-secondary btn-sm">
              Go back
            </Link>
          </div>
        </div>
      </div>
    </div>
  );
}
