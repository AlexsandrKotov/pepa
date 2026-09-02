'use client';

import { useState, useEffect } from 'react';
import { observability, ObservabilitySettings as ObsSettings } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';

export default function ObservabilitySettingsPage() {
  return (
    <PermissionGuard resource="settings" action="read">
      <ObservabilitySettingsContent />
    </PermissionGuard>
  );
}

function ObservabilitySettingsContent() {
  const [form, setForm] = useState<ObsSettings>({
    otel_enabled: false,
    otel_endpoint: '',
    otel_service_name: 'pepa-api',
    otel_sampling_rate: 1.0,
    otel_insecure: true,
    syslog_enabled: false,
    syslog_network: 'udp',
    syslog_address: '',
    syslog_tag: 'pepa',
    syslog_facility: 'local0',
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState<'syslog' | 'otlp' | null>(null);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  useEffect(() => {
    observability.getSettings()
      .then(data => setForm(data))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleSave = async () => {
    setSaving(true);
    setToast(null);
    try {
      await observability.updateSettings(form);
      setToast({ message: 'Observability settings saved', type: 'success' });
    } catch (err) {
      setToast({ message: `Save failed: ${err}`, type: 'error' });
    } finally {
      setSaving(false);
      setTimeout(() => setToast(null), 3000);
    }
  };

  const testSyslog = async () => {
    setTesting('syslog');
    try {
      const res = await observability.testSyslog(form.syslog_network, form.syslog_address);
      setToast({ message: res.message, type: res.status === 'ok' ? 'success' : 'error' });
    } catch (err) {
      setToast({ message: `Test failed: ${err}`, type: 'error' });
    } finally {
      setTesting(null);
    }
  };

  const testOTLP = async () => {
    setTesting('otlp');
    try {
      const res = await observability.testOTLP(form.otel_endpoint, form.otel_insecure);
      setToast({ message: res.message, type: res.status === 'ok' ? 'success' : 'error' });
    } catch (err) {
      setToast({ message: `Test failed: ${err}`, type: 'error' });
    } finally {
      setTesting(null);
    }
  };

  if (loading) return null;

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
            <h1 className="page-title-modern">Observability & Log Export</h1>
            <p className="page-subtitle-modern">Configure syslog forwarding, OTLP/SigNoz, and detailed audit logging</p>
          </div>
        </div>

        {/* OTLP / SigNoz Section */}
        <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="card-header flex items-center justify-between">
            <div>
              <span className="text-[13px] font-medium text-[var(--text-primary)]">OTLP / SigNoz</span>
              <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Send traces and metrics to OpenTelemetry-compatible backends (SigNoz, Jaeger, Grafana Tempo)</p>
            </div>
            <ToggleSwitch enabled={form.otel_enabled} onChange={() => setForm({ ...form, otel_enabled: !form.otel_enabled })} />
          </div>
          <div className="card-body space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="label">OTLP Endpoint</label>
                <input
                  value={form.otel_endpoint}
                  onChange={e => setForm({ ...form, otel_endpoint: e.target.value })}
                  className="input mt-1"
                  placeholder="localhost:4317"
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">gRPC endpoint (e.g. signoz:4317, jaeger:4317)</p>
              </div>
              <div>
                <label className="label">Service Name</label>
                <input
                  value={form.otel_service_name}
                  onChange={e => setForm({ ...form, otel_service_name: e.target.value })}
                  className="input mt-1"
                  placeholder="pepa-api"
                />
              </div>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="label">Sampling Rate</label>
                <input
                  type="number"
                  min="0"
                  max="1"
                  step="0.1"
                  value={form.otel_sampling_rate}
                  onChange={e => setForm({ ...form, otel_sampling_rate: parseFloat(e.target.value) || 1.0 })}
                  className="input mt-1"
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">0.0 = none, 1.0 = all traces</p>
              </div>
              <div className="flex items-end pb-1">
                <label className="flex items-center gap-2 cursor-pointer">
                  <span className="text-[12px] text-[var(--text-secondary)]">Insecure (no TLS)</span>
                  <input
                    type="checkbox"
                    checked={form.otel_insecure}
                    onChange={e => setForm({ ...form, otel_insecure: e.target.checked })}
                    className="rounded border-[var(--border)]"
                  />
                </label>
              </div>
            </div>
            <div className="flex justify-end">
              <button
                onClick={testOTLP}
                disabled={testing === 'otlp' || !form.otel_endpoint}
                className="btn btn-secondary text-[12px] px-3 py-1.5"
              >
                {testing === 'otlp' ? 'Testing...' : 'Test Connection'}
              </button>
            </div>
          </div>
        </div>

        {/* Syslog Section */}
        <div className="card page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
          <div className="card-header flex items-center justify-between">
            <div>
              <span className="text-[13px] font-medium text-[var(--text-primary)]">Syslog Forwarding</span>
              <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Forward all PEPA logs and audit events to a remote syslog server</p>
            </div>
            <ToggleSwitch enabled={form.syslog_enabled} onChange={() => setForm({ ...form, syslog_enabled: !form.syslog_enabled })} />
          </div>
          <div className="card-body space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="label">Network Protocol</label>
                <select
                  value={form.syslog_network}
                  onChange={e => setForm({ ...form, syslog_network: e.target.value })}
                  className="select mt-1"
                >
                  <option value="udp">UDP</option>
                  <option value="tcp">TCP</option>
                </select>
              </div>
              <div>
                <label className="label">Server Address</label>
                <input
                  value={form.syslog_address}
                  onChange={e => setForm({ ...form, syslog_address: e.target.value })}
                  className="input mt-1"
                  placeholder="syslog-server:514"
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">host:port format</p>
              </div>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="label">Tag</label>
                <input
                  value={form.syslog_tag}
                  onChange={e => setForm({ ...form, syslog_tag: e.target.value })}
                  className="input mt-1"
                  placeholder="pepa"
                />
              </div>
              <div>
                <label className="label">Facility</label>
                <select
                  value={form.syslog_facility}
                  onChange={e => setForm({ ...form, syslog_facility: e.target.value })}
                  className="select mt-1"
                >
                  <option value="local0">local0</option>
                  <option value="local1">local1</option>
                  <option value="local2">local2</option>
                  <option value="local3">local3</option>
                  <option value="local4">local4</option>
                  <option value="local5">local5</option>
                  <option value="local6">local6</option>
                  <option value="local7">local7</option>
                  <option value="user">user</option>
                  <option value="daemon">daemon</option>
                </select>
              </div>
            </div>
            <div className="flex justify-end">
              <button
                onClick={testSyslog}
                disabled={testing === 'syslog' || !form.syslog_address}
                className="btn btn-secondary text-[12px] px-3 py-1.5"
              >
                {testing === 'syslog' ? 'Testing...' : 'Test Connection'}
              </button>
            </div>
          </div>
        </div>

        {/* Audit Log Detail Section */}
        <div className="card page-animate-up page-delay-3" style={{ borderRadius: '12px' }}>
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Audit Log Detail</span>
            <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5">Every user action is logged with full detail: page views, clicks, API calls, pipeline launches, deployments</p>
          </div>
          <div className="card-body">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="bg-[var(--bg)] rounded-lg p-3">
                <p className="text-[11px] text-[var(--text-tertiary)]">Log Level</p>
                <p className="text-[13px] font-medium text-[var(--text-primary)]">Detailed</p>
              </div>
              <div className="bg-[var(--bg)] rounded-lg p-3">
                <p className="text-[11px] text-[var(--text-tertiary)]">Tracked Actions</p>
                <p className="text-[13px] font-medium text-[var(--text-primary)]">view, create, update, delete</p>
              </div>
              <div className="bg-[var(--bg)] rounded-lg p-3">
                <p className="text-[11px] text-[var(--text-tertiary)]">Metadata</p>
                <p className="text-[13px] font-medium text-[var(--text-primary)]">path, query, IP, user agent</p>
              </div>
              <div className="bg-[var(--bg)] rounded-lg p-3">
                <p className="text-[11px] text-[var(--text-tertiary)]">Export Targets</p>
                <p className="text-[13px] font-medium text-[var(--text-primary)]">Syslog, OTLP/SigNoz</p>
              </div>
            </div>
          </div>
        </div>

        {/* Save Button */}
        <div className="flex justify-end page-animate-up page-delay-4">
          <button onClick={handleSave} disabled={saving} className="btn btn-primary">
            {saving ? 'Saving...' : 'Save Observability Settings'}
          </button>
        </div>
      </div>
    </div>
  );
}

function ToggleSwitch({ enabled, onChange }: { enabled: boolean; onChange: () => void }) {
  return (
    <div className="flex items-center gap-3">
      <span className="text-[11px] text-[var(--text-tertiary)]">{enabled ? 'Enabled' : 'Disabled'}</span>
      <button
        type="button"
        role="switch"
        aria-checked={enabled}
        onClick={onChange}
        className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 ${enabled ? 'bg-[var(--accent)]' : 'bg-[var(--border-light)]'}`}
      >
        <span className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${enabled ? 'translate-x-4' : 'translate-x-0'}`} />
      </button>
    </div>
  );
}
