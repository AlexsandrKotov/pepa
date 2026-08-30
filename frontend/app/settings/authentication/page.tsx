'use client';

import { useState, useEffect } from 'react';
import { getOIDCAdminConfig, platformSettings } from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';

interface OIDCSettings {
  enabled: boolean;
  issuer: string;
  client_id: string;
  client_secret: string;
  redirect_url: string;
  scopes: string;
}

export default function AuthenticationSettingsPage() {
  return (
    <PermissionGuard resource="settings" action="read">
      <AuthenticationSettingsContent />
    </PermissionGuard>
  );
}

function AuthenticationSettingsContent() {
  const [form, setForm] = useState<OIDCSettings>({
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

  useEffect(() => {
    (async () => {
      try {
        // Always load from the masked admin endpoint — never read
        // client_secret from the generic platformSettings.get('oidc').
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

  const [error, setError] = useState('');

  const handleSave = async () => {
    setSaving(true);
    setToast(null);
    setError('');

    // Validate required fields when SSO is enabled
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
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {toast && (
          <div className={`px-4 py-2.5 rounded-xl text-[13px] page-animate-up ${toast.type === 'success' ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'}`}>
            {toast.message}
          </div>
        )}

        {error && (
          <div className="px-4 py-2.5 rounded-xl text-[13px] page-animate-up bg-red-500/10 text-red-500 border border-red-500/20">
            {error}
          </div>
        )}

        <div className="page-animate">
          <div>
            <h1 className="page-title-modern">Authentication</h1>
            <p className="page-subtitle-modern">Configure SSO and identity provider settings</p>
          </div>
        </div>

        {/* SSO / OIDC Card */}
        <div className="card page-animate-up page-delay-1" style={{ borderRadius: '12px' }}>
          <div className="card-header flex items-center justify-between">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Single Sign-On (SSO / OIDC)</span>
            <div className="flex items-center gap-3">
              <span className="text-[11px] text-[var(--text-tertiary)]">{form.enabled ? 'Enabled' : 'Disabled'}</span>
              <button
                type="button"
                role="switch"
                aria-checked={form.enabled}
                onClick={() => setForm({ ...form, enabled: !form.enabled })}
                className={`relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-2 ${form.enabled ? 'bg-[var(--accent)]' : 'bg-[var(--border-light)]'}`}
              >
                <span className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${form.enabled ? 'translate-x-4' : 'translate-x-0'}`} />
              </button>
            </div>
          </div>
          <div className="card-body space-y-5">
            <div className="p-3 rounded-lg bg-blue-500/5 border border-blue-500/10">
              <p className="text-[11px] text-blue-500/80">
                Configure an OpenID Connect provider to enable SSO login. Users will see a &quot;Sign in with SSO&quot; button on the login page.
                Changes take effect immediately.
              </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
              <div>
                <label className="label">Issuer URL *</label>
                <input
                  value={form.issuer}
                  onChange={e => setForm({ ...form, issuer: e.target.value })}
                  className="input mt-1"
                  placeholder="https://accounts.google.com/.well-known/openid-configuration"
                  disabled={!form.enabled}
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  The OIDC discovery URL or issuer (e.g. <code className="bg-[var(--border-light)] px-1 rounded">https://auth.example.com</code>)
                </p>
              </div>
              <div>
                <label className="label">Client ID *</label>
                <input
                  value={form.client_id}
                  onChange={e => setForm({ ...form, client_id: e.target.value })}
                  className="input mt-1"
                  placeholder="my-app-client-id"
                  disabled={!form.enabled}
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">The client ID from your identity provider</p>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
              <div>
                <label className="label">Client Secret *</label>
                <input
                  type="password"
                  value={form.client_secret}
                  onChange={e => setForm({ ...form, client_secret: e.target.value })}
                  className="input mt-1"
                  placeholder="••••••••"
                  disabled={!form.enabled}
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Leave unchanged to keep the current secret</p>
              </div>
              <div>
                <label className="label">Redirect URL *</label>
                <input
                  value={form.redirect_url}
                  onChange={e => setForm({ ...form, redirect_url: e.target.value })}
                  className="input mt-1"
                  placeholder="http://localhost:8088/api/v1/auth/oidc/callback"
                  disabled={!form.enabled}
                />
                <p className="text-[11px] text-[var(--text-tertiary)] mt-1">
                  Register this URL in your IdP&apos;s allowed callbacks
                </p>
              </div>
            </div>

            <div>
              <label className="label">Scopes</label>
              <input
                value={form.scopes}
                onChange={e => setForm({ ...form, scopes: e.target.value })}
                className="input mt-1"
                placeholder="openid profile email"
                disabled={!form.enabled}
              />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">Space-separated list of OIDC scopes (default: <code className="bg-[var(--border-light)] px-1 rounded">openid profile email</code>)</p>
            </div>
          </div>
          <div className="card-footer flex justify-end">
            <button onClick={handleSave} disabled={saving} className="btn btn-primary btn-sm">
              {saving ? 'Saving...' : 'Save SSO Settings'}
            </button>
          </div>
        </div>

        {/* Help Card */}
        <div className="card page-animate-up page-delay-2" style={{ borderRadius: '12px' }}>
          <div className="card-header">
            <span className="text-[13px] font-medium text-[var(--text-primary)]">Supported Providers</span>
          </div>
          <div className="card-body">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              {[
                { name: 'Keycloak', color: 'text-red-500' },
                { name: 'Auth0', color: 'text-orange-500' },
                { name: 'Google', color: 'text-blue-500' },
                { name: 'Azure AD', color: 'text-sky-500' },
                { name: 'Okta', color: 'text-blue-600' },
                { name: 'GitLab', color: 'text-orange-600' },
                { name: 'GitHub', color: 'text-purple-500' },
                { name: 'Any OIDC', color: 'text-[var(--text-secondary)]' },
              ].map(p => (
                <div key={p.name} className="bg-[var(--bg)] rounded-lg p-3 flex items-center gap-2">
                  <div className={`w-2 h-2 rounded-full ${p.color.replace('text-', 'bg-')}`} />
                  <span className="text-[12px] font-medium text-[var(--text-primary)]">{p.name}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
