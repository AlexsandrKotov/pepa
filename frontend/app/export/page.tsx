'use client';

import { useState } from 'react';
import { services, workflows, entities } from '@/lib/api';
import { Toast } from '@/components/Interactive';
import ConceptHelp from '@/components/ConceptHelp';

type ExportType = 'service' | 'workflow' | 'entity' | 'all';

interface ExportPreview {
  type: ExportType;
  count: number;
  data: unknown;
}

export default function ExportPage() {
  const [exportType, setExportType] = useState<ExportType>('all');
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<ExportPreview | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const handlePreview = async () => {
    setLoading(true);
    setToast(null);
    setPreview(null);

    try {
      let data: unknown;
      let count = 0;

      switch (exportType) {
        case 'service': {
          const res = await services.list({ per_page: '1000' });
          data = res.items || [];
          count = (res.items || []).length;
          break;
        }
        case 'workflow': {
          const res = await workflows.list();
          data = res.workflows || [];
          count = (res.workflows || []).length;
          break;
        }
        case 'entity': {
          const res = await entities.list();
          data = res.items || [];
          count = (res.items || []).length;
          break;
        }
        case 'all': {
          const [svcRes, wfRes, entRes] = await Promise.all([
            services.list({ per_page: '1000' }),
            workflows.list(),
            entities.list(),
          ]);
          const svcItems = svcRes.items || [];
          const wfItems = wfRes.workflows || [];
          const entItems = entRes.items || [];
          data = {
            services: svcItems,
            workflows: wfItems,
            entities: entItems,
            exported_at: new Date().toISOString(),
          };
          count = svcItems.length + wfItems.length + entItems.length;
          break;
        }
      }

      setPreview({ type: exportType, count, data });
      if (count === 0) {
        setToast({ message: 'No items found to export', type: 'error' });
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Export failed';
      setToast({ message, type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  const handleDownload = () => {
    if (!preview) return;

    const json = JSON.stringify(preview.data, null, 2);
    const blob = new Blob([json], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    const timestamp = new Date().toISOString().slice(0, 10);
    a.href = url;
    a.download = `pepa-export-${preview.type}-${timestamp}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    setToast({ message: `Exported ${preview.count} item(s) successfully!`, type: 'success' });
  };

  const handleCopy = async () => {
    if (!preview) return;
    try {
      await navigator.clipboard.writeText(JSON.stringify(preview.data, null, 2));
      setToast({ message: 'Copied to clipboard!', type: 'success' });
    } catch {
      setToast({ message: 'Failed to copy to clipboard', type: 'error' });
    }
  };

  const exportOptions: { type: ExportType; icon: string; label: string; description: string }[] = [
    { type: 'all', icon: '📁', label: 'All Data', description: 'Export everything at once' },
    { type: 'service', icon: '🚀', label: 'Services', description: 'Export service definitions' },
    { type: 'workflow', icon: '⚙️', label: 'Workflows', description: 'Export workflow definitions' },
    { type: 'entity', icon: '📦', label: 'Entities', description: 'Export entity definitions' },
  ];

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

        <div className="page-animate">
          <div className="flex items-center gap-2">
            <h1 className="page-title-modern">Export</h1>
            <ConceptHelp term="export" />
          </div>
          <p className="page-subtitle-modern">
            Export services, workflows, and entities as JSON
          </p>
        </div>

        {/* Export Type Selection */}
        <div className="card card-body page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <label className="label">What would you like to export?</label>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3 mt-2">
            {exportOptions.map((opt) => (
              <button
                key={opt.type}
                onClick={() => { setExportType(opt.type); setPreview(null); }}
                className={`p-4 border-2 rounded-lg text-left transition-all ${
                  exportType === opt.type
                    ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                    : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                }`}
              >
                <div className="text-2xl mb-2">{opt.icon}</div>
                <div className="text-[14px] font-medium text-[var(--text-primary)]">{opt.label}</div>
                <div className="text-[12px] text-[var(--text-secondary)] mt-1">
                  {opt.description}
                </div>
              </button>
            ))}
          </div>
        </div>

        {/* Actions */}
        <div className="flex gap-3 page-animate-up page-delay-2">
          <button
            onClick={handlePreview}
            disabled={loading}
            className="btn btn-primary disabled:opacity-50"
          >
            {loading ? 'Loading...' : 'Generate Export'}
          </button>
          {preview && preview.count > 0 && (
            <>
              <button onClick={handleDownload} className="btn btn-secondary">
                Download JSON
              </button>
              <button onClick={handleCopy} className="btn btn-secondary">
                Copy to Clipboard
              </button>
            </>
          )}
        </div>

        {/* Preview */}
        {preview && preview.count > 0 && (
          <div className="card card-body page-animate-up" style={{ borderRadius: '12px' }}>
            <div className="flex items-center justify-between mb-2">
              <label className="label mb-0">
                Preview ({preview.count} item{preview.count !== 1 ? 's' : ''})
              </label>
            </div>
            <pre
              className="bg-[var(--bg-secondary)] border border-[var(--border)] rounded-lg p-4 text-[12px] font-mono text-[var(--text-secondary)] overflow-auto max-h-[500px]"
              style={{ fontFamily: 'monospace' }}
            >
              {JSON.stringify(preview.data, null, 2)}
            </pre>
          </div>
        )}

        {/* Help Section */}
        <div className="card bg-blue-500/10 border-blue-500/20 page-animate-up page-delay-3" style={{ borderRadius: '12px' }}>
          <div className="card-body">
            <h3 className="text-[13px] font-medium text-blue-500 mb-2">
              About Export
            </h3>
            <div className="text-[12px] text-blue-500 space-y-2">
              <p>Exported data is in JSON format and can be re-imported using the <a href="/import" className="underline hover:no-underline">Import</a> page.</p>
              <p><strong>All Data:</strong> Exports services, workflows, and entities in a single file.</p>
              <p><strong>Individual types:</strong> Export only the data you need for selective migration or backup.</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
