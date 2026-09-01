'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { services, workflows, entities } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConceptHelp from '@/components/ConceptHelp';
import GearIcon from '@/components/GearIcon';

type ImportType = 'service' | 'workflow' | 'entity';

export default function ImportPage() {
  const router = useRouter();
  const [importType, setImportType] = useState<ImportType>('service');
  const [importData, setImportData] = useState('');
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const handleImport = async () => {
    if (!importData.trim()) {
      setToast({ message: 'Please enter data to import', type: 'error' });
      return;
    }

    setLoading(true);
    setToast(null);

    try {
      const data = JSON.parse(importData);

      switch (importType) {
        case 'service':
          await services.create(data);
          setToast({ message: 'Service imported successfully!', type: 'success' });
          break;
        case 'workflow':
          await workflows.create(data);
          setToast({ message: 'Workflow imported successfully!', type: 'success' });
          break;
        case 'entity':
          await entities.create(data);
          setToast({ message: 'Entity imported successfully!', type: 'success' });
          break;
      }

      setImportData('');
      setTimeout(() => router.push(`/${importType}s`), 1500);
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Import failed';
      setToast({ message, type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const examples: Record<ImportType, object> = {
    service: {
      name: 'my-service',
      description: 'My awesome service',
      type: 'web',
      repository: 'https://github.com/org/repo',
      owner: 'team-platform',
    },
    workflow: {
      name: 'deploy-pipeline',
      description: 'CI/CD pipeline for deployment',
      spec: {
        steps: [
          { name: 'build', type: 'action', plugin: 'core', action: 'build' },
          { name: 'test', type: 'action', plugin: 'core', action: 'test' },
          { name: 'deploy', type: 'action', plugin: 'argocd', action: 'deploy' },
        ],
      },
    },
    entity: {
      name: 'production-db',
      type_key: 'database',
      description: 'Production database',
      metadata: {
        engine: 'postgresql',
        version: '14.0',
      },
    },
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      <div className="page-animate">
        <div className="flex items-center gap-2">
          <h1 className="page-title-modern">Import</h1>
          <ConceptHelp term="import" />
        </div>
        <p className="page-subtitle-modern">
          Import services, workflows, or entities from JSON/YAML
        </p>
      </div>

      {/* Import Type Selection */}
      <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
        <label className="label">What would you like to import?</label>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mt-2">
          <button
            onClick={() => setImportType('service')}
            className={`p-4 border-2 rounded-lg text-left transition-all ${
              importType === 'service'
                ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
            }`}
          >
            <div className="text-2xl mb-2">🚀</div>
            <div className="text-[14px] font-medium text-[var(--text-primary)]">Service</div>
            <div className="text-[12px] text-[var(--text-secondary)] mt-1">
              Import a service definition
            </div>
          </button>

          <button
            onClick={() => setImportType('workflow')}
            className={`p-4 border-2 rounded-lg text-left transition-all ${
              importType === 'workflow'
                ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
            }`}
          >
            <div className="text-2xl mb-2"><GearIcon className="w-7 h-7" /></div>
            <div className="text-[14px] font-medium text-[var(--text-primary)]">Workflow</div>
            <div className="text-[12px] text-[var(--text-secondary)] mt-1">
              Import a workflow definition
            </div>
          </button>

          <button
            onClick={() => setImportType('entity')}
            className={`p-4 border-2 rounded-lg text-left transition-all ${
              importType === 'entity'
                ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
            }`}
          >
            <div className="text-2xl mb-2">📦</div>
            <div className="text-[14px] font-medium text-[var(--text-primary)]">Entity</div>
            <div className="text-[12px] text-[var(--text-secondary)] mt-1">
              Import an entity (database, queue, etc.)
            </div>
          </button>
        </div>
      </div>

      {/* Import Data */}
      <div className="card card-body page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
        <div className="flex items-center justify-between mb-2">
          <label className="label mb-0">Import Data (JSON)</label>
          <button
            onClick={() => setImportData(JSON.stringify(examples[importType], null, 2))}
            className="text-[12px] text-[var(--accent)] hover:underline"
          >
            Load Example
          </button>
        </div>
        <textarea
          value={importData}
          onChange={(e) => setImportData(e.target.value)}
          rows={15}
          className="input font-mono text-[12px]"
          placeholder="Paste your JSON data here..."
          style={{ fontFamily: 'monospace' }}
        />
        <p className="text-[11px] text-[var(--text-tertiary)] mt-2">
          💡 Tip: Click {`"Load Example"`} to see the expected format
        </p>
      </div>

      {/* Actions */}
      <div className="flex gap-3">
        <button
          onClick={handleImport}
          disabled={loading || !importData.trim()}
          className="btn btn-primary disabled:opacity-50"
        >
          {loading ? 'Importing...' : 'Import'}
        </button>
        <button
          onClick={() => router.back()}
          className="btn btn-secondary"
        >
          Cancel
        </button>
      </div>

      {/* Help Section */}
      <div className="card bg-blue-500/10 border-blue-500/20 page-animate-up page-delay-3" style={{ borderRadius: '12px' }}>
        <div className="card-body">
          <h3 className="text-[13px] font-medium text-blue-500 mb-2">
            📖 Import Format Guide
          </h3>
          <div className="text-[12px] text-blue-500 space-y-2">
            <p><strong>Service:</strong> Requires name, type, and optional metadata</p>
            <p><strong>Workflow:</strong> Requires name and spec with steps</p>
            <p><strong>Entity:</strong> Requires name, type_key, and optional metadata</p>
          </div>
        </div>
      </div>
      </div>
    </div>
  );
}
