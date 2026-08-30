'use client';

import { useState, useEffect } from 'react';
import { getOIDCAdminConfig, getAzureAdminConfig, getLDAPAdminConfig, testLDAPConnection, platformSettings } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';

type Tab = 'sso' | 'azure' | 'ldap';

export default function AuthenticationSettingsPage() {
  const [tab, setTab] = useState<Tab>('sso');

  return (
    <PermissionGuard resource="settings" action="read">
      <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
        <div className="px-6 py-6 space-y-6">
          <div className="page-animate">
            <h1 className="page-title-modern">Authentication</h1>
            <p className="page-subtitle-modern">Configure identity providers and authentication methods</p>
          </div>

          {/* Tabs */}
          <div className="flex gap-1 p-1 bg-[var(--bg)] rounded-xl border border-[var(--border-light)] w-fit page-animate-up">
            {([
              { id: 'sso' as Tab, label: 'SSO / OIDC' },
              { id: 'azure' as Tab, label: 'Azure AD' },
              { id: 'ldap' as Tab, label: 'LDAP' },
            ]).map(t => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className={`px-4 py-1.5 rounded-lg text-[13px] font-medium transition-all ${
                  tab === t.id
                    ? 'bg-[var(--accent)] text-white shadow-sm'
                    : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-hover)]'
                }`}
              >
                {t.label}
              </button>
            ))}
          </div>

          {tab === 'sso' && <SSOSettingsPanel />}
          {tab === 'azure' && <AzureSettingsPanel />}
          {tab === 'ldap' && <LDAPSettingsPanel />}
        </div>
      </div>
    </PermissionGuard>
  );
}

// ── SSO / OIDC Panel ─────────────────────────────────────────

function SSOSettingsPanel() {
  const [form, setForm] = useState({
    enabled: false,
    issuer: '',
    client_id: '',
    client_secret: '',
    redirect_url: '',
    scopes: 'openid profile email',
  });
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    (async () => {
      try {
        const cfg = await getOIDCAdminConfig();
        setForm({
          enabled: cfg.enabled,
          issuer: cfg.issuer || '',
          client_id: cfg.client_id || '',
          client_secret: cfg.client_secret || '',
          redirect_url: cfg.redirect_url || '',
          scopes: (cfg.scopes || ['openid', 'profile', 'email']).join(' '),
        });
      } catch { /* no config yet */ }
      setLoaded(true);
    })();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    setToast(null);
    setError('');
    if (form.enabled) {
      if (!form.issuer.trim()) { setError('Issuer URL is required'); setSaving(false); return; }
      if (!form.client_id.trim()) { setError('Client ID is required'); setSaving(false); return; }
      if (!form.redirect_url.trim()) { setError('Redirect URL is required'); setSaving(false); return; }
    }
    try {
      const scopesArr = form.scopes.split(/\s+/).filter(Boolean);
      await platformSettings.update('oidc', {
        enabled: form.enabled,
        issuer: form.issuer.trim(),
        client_id: form.client_id.trim(),
        client_secret: form.client_secret,
        redirect_url: form.redirect_url.trim(),
        scopes: scopesArr,
      });
      setToast({ message: 'SSO configuration saved', type: 'success' });
    } catch (err) {
      setToast({ message: `Save failed: ${err}`, type: 'error' });
    } finally {
      setSaving(false);
      setTimeout(() => setToast(null), 3000);
    }
  };

  if (!loaded) return null;

  return (
    <div className="space-y-6">
      <Toast toast={toast} />
      {error && <ErrorBanner message={error} />}

      <div className="card page-animate-up" style={{ borderRadius: '12px' }}>
        <div className="card-header flex items-center justify-between">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Single Sign-On (SSO / OIDC)</span>
          <ToggleSwitch enabled={form.enabled} onChange={() => setForm({ ...form, enabled: !form.enabled })} />
        </div>
        <div className="card-body space-y-5">
          <InfoBanner text="Configure an OpenID Connect provider to enable SSO login. Users will see a &quot;Sign in with SSO&quot; button on the login page." />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Issuer URL *</label>
              <input value={form.issuer} onChange={e => setForm({ ...form, issuer: e.target.value })} className="input mt-1" placeholder="https://accounts.google.com" disabled={!form.enabled} />
            </div>
            <div>
              <label className="label">Client ID *</label>
              <input value={form.client_id} onChange={e => setForm({ ...form, client_id: e.target.value })} className="input mt-1" placeholder="my-app-client-id" disabled={!form.enabled} />
            </div>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Client Secret *</label>
              <input type="password" value={form.client_secret} onChange={e => setForm({ ...form, client_secret: e.target.value })} className="input mt-1" placeholder="••••••••" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Leave unchanged to keep the current secret</p>
            </div>
            <div>
              <label className="label">Redirect URL *</label>
              <input value={form.redirect_url} onChange={e => setForm({ ...form, redirect_url: e.target.value })} className="input mt-1" placeholder="http://localhost:8088/api/v1/auth/oidc/callback" disabled={!form.enabled} />
            </div>
          </div>
          <div>
            <label className="label">Scopes</label>
            <input value={form.scopes} onChange={e => setForm({ ...form, scopes: e.target.value })} className="input mt-1" placeholder="openid profile email" disabled={!form.enabled} />
          </div>
        </div>
        <div className="card-footer flex justify-end">
          <button onClick={handleSave} disabled={saving} className="btn btn-primary btn-sm">{saving ? 'Saving...' : 'Save SSO Settings'}</button>
        </div>
      </div>
    </div>
  );
}

// ── Azure AD Panel ───────────────────────────────────────────

function AzureSettingsPanel() {
  const [form, setForm] = useState({
    enabled: false,
    tenant_id: '',
    client_id: '',
    client_secret: '',
    redirect_url: '',
  });
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    (async () => {
      try {
        const cfg = await getAzureAdminConfig();
        setForm({
          enabled: cfg.enabled,
          tenant_id: cfg.tenant_id || '',
          client_id: cfg.client_id || '',
          client_secret: cfg.client_secret || '',
          redirect_url: cfg.redirect_url || '',
        });
      } catch { /* no config yet */ }
      setLoaded(true);
    })();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    setToast(null);
    setError('');
    if (form.enabled) {
      if (!form.tenant_id.trim()) { setError('Tenant ID is required'); setSaving(false); return; }
      if (!form.client_id.trim()) { setError('Client ID (Application ID) is required'); setSaving(false); return; }
      if (!form.redirect_url.trim()) { setError('Redirect URL is required'); setSaving(false); return; }
    }
    try {
      await platformSettings.update('azure_ad', {
        enabled: form.enabled,
        tenant_id: form.tenant_id.trim(),
        client_id: form.client_id.trim(),
        client_secret: form.client_secret,
        redirect_url: form.redirect_url.trim(),
      });
      setToast({ message: 'Azure AD configuration saved', type: 'success' });
    } catch (err) {
      setToast({ message: `Save failed: ${err}`, type: 'error' });
    } finally {
      setSaving(false);
      setTimeout(() => setToast(null), 3000);
    }
  };

  if (!loaded) return null;

  return (
    <div className="space-y-6">
      <Toast toast={toast} />
      {error && <ErrorBanner message={error} />}

      <div className="card page-animate-up" style={{ borderRadius: '12px' }}>
        <div className="card-header flex items-center justify-between">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">Azure AD (Microsoft Entra ID)</span>
          <ToggleSwitch enabled={form.enabled} onChange={() => setForm({ ...form, enabled: !form.enabled })} />
        </div>
        <div className="card-body space-y-5">
          <InfoBanner text="Configure Azure AD to enable Microsoft account SSO. Uses the standard OIDC flow with Microsoft Identity Platform v2.0 endpoints." />

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Tenant ID *</label>
              <input value={form.tenant_id} onChange={e => setForm({ ...form, tenant_id: e.target.value })} className="input mt-1" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Use &quot;common&quot; for multi-tenant, or a specific tenant ID for single-tenant</p>
            </div>
            <div>
              <label className="label">Application (Client) ID *</label>
              <input value={form.client_id} onChange={e => setForm({ ...form, client_id: e.target.value })} className="input mt-1" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" disabled={!form.enabled} />
            </div>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Client Secret *</label>
              <input type="password" value={form.client_secret} onChange={e => setForm({ ...form, client_secret: e.target.value })} className="input mt-1" placeholder="••••••••" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Leave unchanged to keep the current secret</p>
            </div>
            <div>
              <label className="label">Redirect URL *</label>
              <input value={form.redirect_url} onChange={e => setForm({ ...form, redirect_url: e.target.value })} className="input mt-1" placeholder="http://localhost:8088/api/v1/auth/azure/callback" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Register this URL in Azure App Registration &gt; Authentication</p>
            </div>
          </div>
        </div>
        <div className="card-footer flex justify-end">
          <button onClick={handleSave} disabled={saving} className="btn btn-primary btn-sm">{saving ? 'Saving...' : 'Save Azure AD Settings'}</button>
        </div>
      </div>
    </div>
  );
}

// ── LDAP Panel ───────────────────────────────────────────────

function LDAPSettingsPanel() {
  const [form, setForm] = useState({
    enabled: false,
    url: '',
    bind_dn: '',
    bind_password: '',
    base_dn: '',
    user_filter: '(&(objectClass=person)(mail=%s))',
    group_filter: '(&(objectClass=group)(member=%s))',
    email_attr: 'mail',
    name_attr: 'cn',
    start_tls: false,
    insecure_skip_verify: false,
    group_mappings: [] as { group: string; role: string }[],
  });
  const [saving, setSaving] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [error, setError] = useState('');
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ status: string; message: string } | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const cfg = await getLDAPAdminConfig();
        const mappings = cfg.group_mapping
          ? Object.entries(cfg.group_mapping).map(([group, role]) => ({ group, role }))
          : [];
        setForm({
          enabled: cfg.enabled,
          url: cfg.url || '',
          bind_dn: cfg.bind_dn || '',
          bind_password: cfg.bind_password || '',
          base_dn: cfg.base_dn || '',
          user_filter: cfg.user_filter || '(&(objectClass=person)(mail=%s))',
          group_filter: cfg.group_filter || '(&(objectClass=group)(member=%s))',
          email_attr: cfg.email_attr || 'mail',
          name_attr: cfg.name_attr || 'cn',
          start_tls: cfg.start_tls || false,
          insecure_skip_verify: cfg.insecure_skip_verify || false,
          group_mappings: mappings,
        });
      } catch { /* no config yet */ }
      setLoaded(true);
    })();
  }, []);

  const handleSave = async () => {
    setSaving(true);
    setToast(null);
    setError('');
    if (form.enabled) {
      if (!form.url.trim()) { setError('Server URL is required'); setSaving(false); return; }
      if (!form.base_dn.trim()) { setError('Base DN is required'); setSaving(false); return; }
    }
    try {
      const groupMapping: Record<string, string> = {};
      for (const m of form.group_mappings) {
        if (m.group.trim() && m.role.trim()) {
          groupMapping[m.group.trim()] = m.role.trim();
        }
      }
      await platformSettings.update('ldap', {
        enabled: form.enabled,
        url: form.url.trim(),
        bind_dn: form.bind_dn.trim(),
        bind_password: form.bind_password,
        base_dn: form.base_dn.trim(),
        user_filter: form.user_filter.trim(),
        group_filter: form.group_filter.trim(),
        email_attr: form.email_attr.trim(),
        name_attr: form.name_attr.trim(),
        start_tls: form.start_tls,
        insecure_skip_verify: form.insecure_skip_verify,
        group_mapping: groupMapping,
      });
      setToast({ message: 'LDAP configuration saved', type: 'success' });
    } catch (err) {
      setToast({ message: `Save failed: ${err}`, type: 'error' });
    } finally {
      setSaving(false);
      setTimeout(() => setToast(null), 3000);
    }
  };

  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testLDAPConnection({
        url: form.url,
        bind_dn: form.bind_dn,
        bind_password: form.bind_password,
        base_dn: form.base_dn,
        start_tls: form.start_tls,
        insecure_skip_verify: form.insecure_skip_verify,
      });
      setTestResult(result);
    } catch (err) {
      setTestResult({ status: 'error', message: String(err) });
    } finally {
      setTesting(false);
    }
  };

  const addGroupMapping = () => {
    setForm({ ...form, group_mappings: [...form.group_mappings, { group: '', role: '' }] });
  };

  const removeGroupMapping = (idx: number) => {
    setForm({ ...form, group_mappings: form.group_mappings.filter((_, i) => i !== idx) });
  };

  const updateGroupMapping = (idx: number, field: 'group' | 'role', value: string) => {
    const updated = [...form.group_mappings];
    updated[idx] = { ...updated[idx], [field]: value };
    setForm({ ...form, group_mappings: updated });
  };

  if (!loaded) return null;

  return (
    <div className="space-y-6">
      <Toast toast={toast} />
      {error && <ErrorBanner message={error} />}

      <div className="card page-animate-up" style={{ borderRadius: '12px' }}>
        <div className="card-header flex items-center justify-between">
          <span className="text-[13px] font-medium text-[var(--text-primary)]">LDAP / Active Directory</span>
          <ToggleSwitch enabled={form.enabled} onChange={() => setForm({ ...form, enabled: !form.enabled })} />
        </div>
        <div className="card-body space-y-5">
          <InfoBanner text="Configure LDAP to allow authentication against an Active Directory or other LDAP directory. Users will authenticate with their LDAP credentials." />

          {/* Connection settings */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Server URL *</label>
              <input value={form.url} onChange={e => setForm({ ...form, url: e.target.value })} className="input mt-1" placeholder="ldap://dc.example.com:389" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Use <code className="bg-[var(--border-light)] px-1 rounded">ldap://</code> or <code className="bg-[var(--border-light)] px-1 rounded">ldaps://</code></p>
            </div>
            <div>
              <label className="label">Base DN *</label>
              <input value={form.base_dn} onChange={e => setForm({ ...form, base_dn: e.target.value })} className="input mt-1" placeholder="dc=example,dc=com" disabled={!form.enabled} />
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Bind DN (Service Account)</label>
              <input value={form.bind_dn} onChange={e => setForm({ ...form, bind_dn: e.target.value })} className="input mt-1" placeholder="cn=admin,dc=example,dc=com" disabled={!form.enabled} />
            </div>
            <div>
              <label className="label">Bind Password</label>
              <input type="password" value={form.bind_password} onChange={e => setForm({ ...form, bind_password: e.target.value })} className="input mt-1" placeholder="••••••••" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Leave unchanged to keep the current password</p>
            </div>
          </div>

          {/* Search filters */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">User Filter</label>
              <input value={form.user_filter} onChange={e => setForm({ ...form, user_filter: e.target.value })} className="input mt-1 font-mono text-[12px]" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Use <code className="bg-[var(--border-light)] px-1 rounded">%s</code> as placeholder for email</p>
            </div>
            <div>
              <label className="label">Group Filter</label>
              <input value={form.group_filter} onChange={e => setForm({ ...form, group_filter: e.target.value })} className="input mt-1 font-mono text-[12px]" disabled={!form.enabled} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Use <code className="bg-[var(--border-light)] px-1 rounded">%s</code> as placeholder for user DN</p>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <div>
              <label className="label">Email Attribute</label>
              <input value={form.email_attr} onChange={e => setForm({ ...form, email_attr: e.target.value })} className="input mt-1" placeholder="mail" disabled={!form.enabled} />
            </div>
            <div>
              <label className="label">Name Attribute</label>
              <input value={form.name_attr} onChange={e => setForm({ ...form, name_attr: e.target.value })} className="input mt-1" placeholder="cn" disabled={!form.enabled} />
            </div>
          </div>

          {/* TLS options */}
          <div className="flex items-center gap-6">
            <label className="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" checked={form.start_tls} onChange={e => setForm({ ...form, start_tls: e.target.checked })} disabled={!form.enabled} className="rounded border-[var(--border-light)]" />
              <span className="text-[12px] text-[var(--text-secondary)]">StartTLS</span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" checked={form.insecure_skip_verify} onChange={e => setForm({ ...form, insecure_skip_verify: e.target.checked })} disabled={!form.enabled} className="rounded border-[var(--border-light)]" />
              <span className="text-[12px] text-[var(--text-secondary)]">Skip TLS verification</span>
            </label>
          </div>

          {/* Test connection */}
          <div className="flex items-center gap-3">
            <button onClick={handleTest} disabled={testing || !form.enabled} className="btn btn-sm border border-[var(--border-light)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]">
              {testing ? 'Testing...' : 'Test Connection'}
            </button>
            {testResult && (
              <span className={`text-[12px] ${testResult.status === 'connected' ? 'text-emerald-500' : 'text-red-500'}`}>
                {testResult.message}
              </span>
            )}
          </div>

          {/* Group → Role mapping */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="label mb-0">Group &rarr; Role Mapping</label>
              <button onClick={addGroupMapping} disabled={!form.enabled} className="text-[11px] text-[var(--accent)] hover:underline">+ Add mapping</button>
            </div>
            {form.group_mappings.length === 0 && (
              <p className="text-[11px] text-[var(--text-tertiary)]">No group mappings configured. Add mappings to assign PEPA roles based on LDAP group membership.</p>
            )}
            <div className="space-y-2">
              {form.group_mappings.map((m, idx) => (
                <div key={idx} className="flex items-center gap-2">
                  <input value={m.group} onChange={e => updateGroupMapping(idx, 'group', e.target.value)} className="input flex-1 text-[12px]" placeholder="CN=Admins,DC=example,DC=com" disabled={!form.enabled} />
                  <span className="text-[11px] text-[var(--text-tertiary)]">&rarr;</span>
                  <input value={m.role} onChange={e => updateGroupMapping(idx, 'role', e.target.value)} className="input w-32 text-[12px]" placeholder="admin" disabled={!form.enabled} />
                  <button onClick={() => removeGroupMapping(idx)} disabled={!form.enabled} className="text-red-400 hover:text-red-500 p-1">
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>
        <div className="card-footer flex justify-end">
          <button onClick={handleSave} disabled={saving} className="btn btn-primary btn-sm">{saving ? 'Saving...' : 'Save LDAP Settings'}</button>
        </div>
      </div>
    </div>
  );
}

// ── Shared components ────────────────────────────────────────

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

function InfoBanner({ text }: { text: string }) {
  return (
    <div className="p-3 rounded-lg bg-blue-500/5 border border-blue-500/10">
      <p className="text-[11px] text-blue-500/80">{text} Changes take effect immediately.</p>
    </div>
  );
}

function Toast({ toast }: { toast: { message: string; type: 'success' | 'error' } | null }) {
  if (!toast) return null;
  return (
    <div className={`px-4 py-2.5 rounded-xl text-[13px] page-animate-up ${toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'}`}>
      {toast.message}
    </div>
  );
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="px-4 py-2.5 rounded-xl text-[13px] page-animate-up bg-red-500/10 text-red-500 border border-red-500/20">
      {message}
    </div>
  );
}
