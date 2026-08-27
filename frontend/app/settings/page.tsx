'use client';

import { useState, useEffect } from 'react';
import { platformSettings } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';

interface GeneralSettings {
  platform_name: string;
  base_url: string;
  log_level: string;
  cors_origins: string;
}

export default function GeneralSettingsPage() {
  return (
    <PermissionGuard resource="settings" action="read">
      <GeneralSettingsContent />
    </PermissionGuard>
  );
}

function GeneralSettingsContent() {
  const [form, setForm] = useState<GeneralSettings>({
    platform_name: 'PEPA',
    base_url: 'http://localhost:8088',
    log_level: 'info',
    cors_origins: 'http://localhost:3000',
  });
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const data = await platformSettings.get('general');
        if (data.value) {
          setForm(prev => ({ ...prev, ...data.value }));
        }
      } catch { /* no settings yet */ }
      setLoaded(true);
    })();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    setToast(null);
    try {
      await platformSettings.update('general', form);
      setToast({ message: 'Settings saved', type: 'success' });
    } catch (err) {
      setToast({ message: `Save failed: ${err}`, type: 'error' });
    } finally {
      setSaving(false);
      setTimeout(() => setToast(null), 3000);
    }
  };

  if (!loaded) return null;

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      {toast && (
        <div className={`px-4 py-2.5 rounded-xl text-[13px] page-animate-up ${toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'}`}>
          {toast.message}
        </div>
      )}

      <div className="page-animate">
        <div>
          <h1 className="page-title-modern">Settings</h1>
          <p className="page-subtitle-modern">Platform configuration and system info</p>
        </div>
      </div>

      <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
        <div className="card-header">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Platform Configuration</span>
        </div>
        <div className="card-body space-y-5">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Platform Name</label>
              <input
                value={form.platform_name}
                onChange={e => setForm({ ...form, platform_name: e.target.value })}
                className="input mt-1"
                placeholder="PEPA"
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Displayed in the sidebar and page titles</p>
            </div>
            <div>
              <label className="label">Base URL</label>
              <input
                value={form.base_url}
                onChange={e => setForm({ ...form, base_url: e.target.value })}
                className="input mt-1"
                placeholder="http://localhost:8088"
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">API server URL used by plugins and webhooks</p>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Log Level</label>
              <select
                value={form.log_level}
                onChange={e => setForm({ ...form, log_level: e.target.value })}
                className="select mt-1"
              >
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warn</option>
                <option value="error">Error</option>
              </select>
            </div>
            <div>
              <label className="label">CORS Origins</label>
              <input
                value={form.cors_origins}
                onChange={e => setForm({ ...form, cors_origins: e.target.value })}
                className="input mt-1"
                placeholder="http://localhost:3000"
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Comma-separated list of allowed origins</p>
            </div>
          </div>
        </div>
        <div className="card-footer flex justify-end">
          <button onClick={handleSave} disabled={saving} className="btn btn-primary btn-sm">
            {saving ? 'Saving...' : 'Save Settings'}
          </button>
        </div>
      </div>

      <div className="card page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
        <div className="card-header">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">System Info</span>
        </div>
        <div className="card-body">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="bg-[var(--bg)] rounded-lg p-3">
              <p className="text-[11px] text-[var(--text-tertiary)]">Version</p>
              <p className="text-[13px] font-medium text-[var(--text-primary)]">0.1.0</p>
            </div>
            <div className="bg-[var(--bg)] rounded-lg p-3">
              <p className="text-[11px] text-[var(--text-tertiary)]">Environment</p>
              <p className="text-[13px] font-medium text-[var(--text-primary)]">Development</p>
            </div>
            <div className="bg-[var(--bg)] rounded-lg p-3">
              <p className="text-[11px] text-[var(--text-tertiary)]">Database</p>
              <p className="text-[13px] font-medium text-[var(--text-primary)]">PostgreSQL + PGvector</p>
            </div>
            <div className="bg-[var(--bg)] rounded-lg p-3">
              <p className="text-[11px] text-[var(--text-tertiary)]">Cache</p>
              <p className="text-[13px] font-medium text-[var(--text-primary)]">Redis</p>
            </div>
          </div>
        </div>
      </div>
      </div>
    </div>
  );
}
