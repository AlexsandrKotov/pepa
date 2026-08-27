'use client';
import { useState, useEffect } from 'react';
import { useSearchParams } from 'next/navigation';
import { Suspense } from 'react';
import EnvironmentsClient from './EnvironmentsClient';
import EnvironmentDetailClient from './[id]/EnvironmentDetailClient';
import { environments, type Environment, type EnvironmentContents, type EnvVariable } from '@/lib/api';

function EnvironmentDetailFetcher({ envId }: { envId: string }) {
  const [env, setEnv] = useState<Environment | null>(null);
  const [contents, setContents] = useState<EnvironmentContents | null>(null);
  const [variables, setVariables] = useState<EnvVariable[]>([]);
  const [allEnvs, setAllEnvs] = useState<Array<{ id: string; name: string; slug: string; color: string }>>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      environments.get(envId).catch(() => null),
      environments.contents(envId).catch(() => null),
      environments.variables(envId).catch(() => ({ variables: [] })),
      environments.list().catch(() => ({ environments: [] })),
    ]).then(([envData, contentsData, varsData, envsData]) => {
      setEnv(envData);
      setContents(contentsData);
      setVariables(varsData.variables || []);
      setAllEnvs((envsData.environments || []).map((e) => ({ id: e.id, name: e.name, slug: e.slug || '', color: e.color || '#6B7280' })));
    }).finally(() => setLoading(false));
  }, [envId]);

  if (loading) return <div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>;
  if (!env) return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6">
        <div className="card card-body text-center py-12">
          <h3 className="text-[14px] font-medium text-[var(--text-primary)] mb-1">Environment not found</h3>
          <p className="text-[12px] text-[var(--text-tertiary)]">The environment you are looking for does not exist or has been removed.</p>
        </div>
      </div>
    </div>
  );
  return <EnvironmentDetailClient environment={env} contents={contents} variables={variables} allEnvironments={allEnvs} />;
}

function EnvironmentsPageContent() {
  const searchParams = useSearchParams();
  const envId = searchParams.get('id');

  if (envId) {
    return <EnvironmentDetailFetcher envId={envId} />;
  }
  return <EnvironmentsClient />;
}

export default function EnvironmentsPage() {
  return (
    <Suspense fallback={<div className="flex items-center justify-center py-12"><div className="loading-spinner" /></div>}>
      <EnvironmentsPageContent />
    </Suspense>
  );
}
