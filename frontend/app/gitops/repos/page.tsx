'use client';
import { Suspense } from 'react';
import { useSearchParams } from 'next/navigation';
import RepoDetailClient from './RepoDetailClient';

function GitOpsReposPageContent() {
  const searchParams = useSearchParams();
  const repoId = searchParams.get('id');

  if (repoId) {
    return <RepoDetailClient />;
  }

  // List view - redirect to gitops page
  return <GitOpsReposList />;
}

function GitOpsReposList() {
  // Simple list that links to detail via query param
  return (
    <div className="flex items-center justify-center py-12">
      <p className="text-sm text-[var(--text-tertiary)]">
        Select a repository from the <a href="/gitops" className="text-[var(--accent)]">GitOps</a> page.
      </p>
    </div>
  );
}

export default function GitOpsReposPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <GitOpsReposPageContent />
    </Suspense>
  );
}
