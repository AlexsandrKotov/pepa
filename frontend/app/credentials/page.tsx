'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { listMyCredentials, createMyCredential, updateMyCredential, deleteMyCredential, verifyMyCredential, fetchUserInfoForCredential, listSharedCredentials, shareCredential, listCredentialShares, revokeCredentialShare, listUsers, listTeams, type UserCredential, type SharedCredential, type CredentialShareEntry, type User, type Team } from '@/lib/api';
import { VaultInput, VaultPickerModal, useVaultPicker } from '@/components/VaultInput';
import ConfirmModal from '@/components/ConfirmModal';

const PROVIDERS = [
  { value: 'gitlab', label: 'GitLab' },
  { value: 'github', label: 'GitHub' },
  { value: 'gitea', label: 'Gitea' },
  { value: 'bitbucket', label: 'Bitbucket' },
  { value: 'docker_registry', label: 'Docker Registry' },
  { value: 's3', label: 'S3 / MinIO' },
];

export default function CredentialsPage() {
  const [creds, setCreds] = useState<UserCredential[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [error, setError] = useState('');
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(null);
  const [sharedCreds, setSharedCreds] = useState<SharedCredential[]>([]);
  const [shareModalCred, setShareModalCred] = useState<UserCredential | null>(null);
  const { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef } = useVaultPicker();
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => { loadCreds(); loadSharedCreds(); }, []);

  async function loadCreds() {
    setLoading(true);
    try {
      const data = await listMyCredentials();
      setCreds(data.credentials);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    } finally {
      setLoading(false);
    }
  }

  async function loadSharedCreds() {
    try {
      const data = await listSharedCredentials();
      setSharedCreds(data.credentials || []);
    } catch { /* ignore */ }
  }

  async function handleDelete(id: string) {
    setDeleteConfirm(id);
  }

  async function confirmDelete() {
    if (!deleteConfirm) return;
    setDeleting(true);
    try {
      await deleteMyCredential(deleteConfirm);
      loadCreds();
      setFeedback({ ok: true, text: 'Credential deleted' });
    } catch (err) {
      setFeedback({ ok: false, text: err instanceof Error ? err.message : 'Failed' });
    }
    setDeleting(false);
    setDeleteConfirm(null);
  }

  async function handleVerify(id: string) {
    try {
      const result = await verifyMyCredential(id);
      const ok = result.status === 'connected' || result.status === 'ok' || result.status === 'healthy';
      setFeedback({ ok, text: `${result.status}: ${result.message}` });
      if (result.status === 'connected') loadCreds();
    } catch (err) {
      setFeedback({ ok: false, text: err instanceof Error ? err.message : 'Failed' });
    }
  }

  async function handleToggleDefault(cred: UserCredential) {
    try {
      await updateMyCredential(cred.id, { is_default: !cred.is_default });
      loadCreds();
    } catch (err) {
      setFeedback({ ok: false, text: err instanceof Error ? err.message : 'Failed' });
    }
  }

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
      <div className="page-animate">
        <div>
          <h1 className="page-title-modern">My Credentials</h1>
          <p className="page-subtitle-modern">Manage your personal access tokens for external services.</p>
        </div>
      </div>

      {error && <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded-xl text-sm page-animate-up">{error}</div>}

      {feedback && (
        <div className={`rounded-xl border p-4 flex items-start justify-between gap-3 page-animate-up ${
          feedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
        }`}>
          <div>
            <p className={`text-sm font-medium ${feedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {feedback.ok ? '✓ ' : '⚠ '}{feedback.text}
            </p>
          </div>
          <button onClick={() => setFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
        </div>
      )}

      <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-4 page-animate-up page-delay-1">
        <p className="text-sm text-blue-500">
          <strong>How it works:</strong> When you perform git operations (commits, pushes) or browse S3 storage, PEPA uses your personal credentials to authenticate.
          This ensures commits are authored under your account and S3 access uses your own keys. Global connection tokens are only used as fallback.
        </p>
      </div>

      <div className="flex justify-end page-animate-up page-delay-2">
        <button onClick={() => setShowAdd(true)} className="btn btn-primary btn-sm">
          + Add Credential
        </button>
      </div>

      {loading ? (
        <div className="text-center py-12 text-[var(--text-tertiary)]">Loading...</div>
      ) : creds.length === 0 ? (
        <div className="card" style={{ borderRadius: '12px' }}>
          <div className="card-body text-center text-[var(--text-tertiary)] py-12">
            No credentials configured yet. Add a personal access token to start working with external services.
          </div>
        </div>
      ) : (
        <div className="space-y-3">
          {creds.map(cred => (
            <div key={cred.id} className="card modern-card-hover" style={{ borderRadius: '12px' }}>
              <div className="card-body flex items-center justify-between">
                <div className="flex items-center space-x-3">
                  <span className="bg-[var(--border-light)] text-[var(--text-secondary)] px-2 py-1 rounded text-xs font-medium uppercase">
                    {cred.provider}
                  </span>
                  <div>
                    <div className="text-sm font-medium text-[var(--text-primary)]">{cred.display_name || cred.provider_url}</div>
                    <div className="text-xs text-[var(--text-tertiary)]">{cred.provider_url} &middot; {cred.token_masked}</div>
                    {cred.username && cred.provider !== 's3' && <div className="text-xs text-[var(--text-tertiary)]">Remote user: {cred.username} ({cred.email})</div>}
                    {cred.username && cred.provider === 's3' && <div className="text-xs text-[var(--text-tertiary)]">Access key: {cred.username}</div>}
                  </div>
                </div>
                <div className="flex items-center space-x-2">
                  {cred.is_default && (
                    <span className="bg-blue-500/10 text-blue-500 px-2 py-1 rounded text-xs">Default</span>
                  )}
                  {cred.last_verified && (
                    <span className="text-xs text-[var(--text-tertiary)]">Verified: {new Date(cred.last_verified).toLocaleDateString()}</span>
                  )}
                  <button onClick={() => handleVerify(cred.id)} className="text-emerald-600 text-xs hover:text-emerald-500">Verify</button>
                  <button onClick={() => handleToggleDefault(cred)} className="text-[var(--accent)] text-xs hover:text-[var(--accent)]">
                    {cred.is_default ? 'Unset Default' : 'Set Default'}
                  </button>
                  <button onClick={() => setShareModalCred(cred)} className="text-violet-500 text-xs hover:text-violet-400">Share</button>
                  <button onClick={() => handleDelete(cred.id)} className="text-red-500 text-xs hover:text-red-400">Delete</button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Shared with me section */}
      {sharedCreds.length > 0 && (
        <div className="space-y-3">
          <h2 className="text-[15px] font-medium text-[var(--text-primary)]">Shared with Me</h2>
          <p className="text-[12px] text-[var(--text-tertiary)]">Credentials other users have shared with you.</p>
          <div className="space-y-2">
            {sharedCreds.map(cred => (
              <div key={cred.id} className="card modern-card-hover" style={{ borderRadius: '12px' }}>
                <div className="card-body flex items-center justify-between">
                  <div className="flex items-center space-x-3">
                    <span className="bg-violet-500/10 text-violet-500 px-2 py-1 rounded text-xs font-medium uppercase">
                      {cred.provider}
                    </span>
                    <div>
                      <div className="text-sm font-medium text-[var(--text-primary)]">{cred.display_name || cred.provider_url}</div>
                      <div className="text-xs text-[var(--text-tertiary)]">Shared by {cred.owner_name} ({cred.owner_email})</div>
                      <div className="text-xs text-[var(--text-tertiary)]">{cred.provider_url} &middot; {cred.token_masked}</div>
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {shareModalCred && <ShareCredentialModal cred={shareModalCred} onClose={() => setShareModalCred(null)} />}
      {showAdd && <AddCredentialModal onClose={() => { setShowAdd(false); setVaultRefs({}); }} onAdded={loadCreds} vaultRefs={vaultRefs} onOpenVaultPicker={onOpenVaultPicker} removeVaultRef={removeVaultRef} />}
      {VaultPicker}

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteConfirm}
        title="Delete this credential?"
        description="This credential will be permanently removed. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm(null)}
      />
      </div>
    </div>
  );
}

function ShareCredentialModal({ cred, onClose }: { cred: UserCredential; onClose: () => void }) {
  useEscapeKey(onClose);
  const [shares, setShares] = useState<CredentialShareEntry[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [shareType, setShareType] = useState<'user' | 'team'>('user');
  const [selectedUserId, setSelectedUserId] = useState('');
  const [selectedTeamId, setSelectedTeamId] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    listCredentialShares(cred.id).then(r => setShares(r.shares || [])).catch(() => {});
    listUsers().then(r => setUsers(r.users || [])).catch(() => {});
    listTeams().then(r => setTeams(r.teams || [])).catch(() => {});
  }, [cred.id]);

  async function handleShare() {
    const target = shareType === 'user' ? selectedUserId : selectedTeamId;
    if (!target) { setError('Select a target'); return; }
    setError('');
    setSaving(true);
    try {
      const body = shareType === 'user' ? { shared_with_user: target } : { shared_with_team: target };
      await shareCredential(cred.id, body);
      const res = await listCredentialShares(cred.id);
      setShares(res.shares || []);
      setSelectedUserId('');
      setSelectedTeamId('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    } finally {
      setSaving(false);
    }
  }

  async function handleRevoke(shareId: string) {
    try {
      await revokeCredentialShare(cred.id, shareId);
      setShares(prev => prev.filter(s => s.id !== shareId));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between sticky top-0 bg-[var(--surface)] z-10">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Share: {cred.display_name || cred.provider_url}</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <div className="p-5 space-y-4">
          {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}

          {/* Share form */}
          <div className="space-y-3">
            <div className="flex gap-2">
              <button onClick={() => setShareType('user')} className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${shareType === 'user' ? 'bg-violet-500/15 text-violet-500 border border-violet-500/30' : 'bg-[var(--bg-secondary)] text-[var(--text-tertiary)] border border-[var(--border)]'}`}>User</button>
              <button onClick={() => setShareType('team')} className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors ${shareType === 'team' ? 'bg-violet-500/15 text-violet-500 border border-violet-500/30' : 'bg-[var(--bg-secondary)] text-[var(--text-tertiary)] border border-[var(--border)]'}`}>Team</button>
            </div>
            {shareType === 'user' ? (
              <select className="input" value={selectedUserId} onChange={e => setSelectedUserId(e.target.value)}>
                <option value="">Select user...</option>
                {users.filter(u => u.id !== cred.user_id).map(u => (
                  <option key={u.id} value={u.id}>{u.name} ({u.email})</option>
                ))}
              </select>
            ) : (
              <select className="input" value={selectedTeamId} onChange={e => setSelectedTeamId(e.target.value)}>
                <option value="">Select team...</option>
                {teams.map(t => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            )}
            <button onClick={handleShare} disabled={saving} className="btn btn-primary btn-sm w-full">
              {saving ? 'Sharing...' : 'Share'}
            </button>
          </div>

          {/* Existing shares */}
          <div className="space-y-2">
            <h3 className="text-[13px] font-medium text-[var(--text-secondary)]">Current shares</h3>
            {shares.length === 0 && (
              <p className="text-[12px] text-[var(--text-tertiary)]">Not shared with anyone yet.</p>
            )}
            {shares.map(s => (
              <div key={s.id} className="flex items-center justify-between bg-[var(--bg-secondary)] rounded-lg px-3 py-2">
                <div className="text-[12px] text-[var(--text-secondary)]">
                  {s.shared_with_user ? (
                    <span>{s.shared_with_user.name} ({s.shared_with_user.email})</span>
                  ) : s.shared_with_team ? (
                    <span>Team: {s.shared_with_team.name}</span>
                  ) : (
                    <span>Unknown</span>
                  )}
                  <span className="text-[var(--text-tertiary)] ml-2">{new Date(s.created_at).toLocaleDateString()}</span>
                </div>
                <button onClick={() => handleRevoke(s.id)} className="text-red-500 text-[11px] hover:text-red-400">Revoke</button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function AddCredentialModal({ onClose, onAdded, vaultRefs, onOpenVaultPicker, removeVaultRef }: { onClose: () => void; onAdded: () => void; vaultRefs: Record<string, string>; onOpenVaultPicker: (field: string) => void; removeVaultRef: (field: string) => void }) {
  useEscapeKey(onClose);
  const [provider, setProvider] = useState('gitlab');
  const [providerUrl, setProviderUrl] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [token, setToken] = useState('');
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [isDefault, setIsDefault] = useState(true);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [fetching, setFetching] = useState(false);
  const [fetched, setFetched] = useState(false);

  const defaultUrls: Record<string, string> = {
    gitlab: 'https://gitlab.com',
    github: 'https://api.github.com',
    gitea: '',
    bitbucket: 'https://api.bitbucket.org',
    docker_registry: '',
    s3: '',
  };

  const isS3 = provider === 's3';

  function handleProviderChange(p: string) {
    setProvider(p);
    if (!providerUrl) setProviderUrl(defaultUrls[p] || '');
    setFetched(false);
  }

  async function handleFetchUserInfo() {
    const tokenValue = vaultRefs.token || token;
    if (!tokenValue) return;
    const url = providerUrl || defaultUrls[provider] || '';
    if (!url) {
      setError(isS3 ? 'S3 Endpoint is required to test connection' : 'Instance URL is required to fetch user info');
      return;
    }
    setFetching(true);
    setError('');
    try {
      const info = await fetchUserInfoForCredential(provider, url, tokenValue, isS3 ? username : undefined);
      if (info.username) setUsername(info.username);
      if (info.email) setEmail(info.email);
      setFetched(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch user info');
      setFetched(false);
    } finally {
      setFetching(false);
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setSaving(true);
    try {
      await createMyCredential({
        provider,
        provider_url: providerUrl || defaultUrls[provider],
        display_name: displayName,
        token: vaultRefs.token || token,
        username,
        email,
        is_default: isDefault,
      });
      onAdded();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed');
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto">
        <div className="px-5 py-3 border-b border-[var(--border)] flex items-center justify-between sticky top-0 bg-[var(--surface)] z-10">
          <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">Add Credential</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="p-5 space-y-4">
            {error && <div className="p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-[12px] text-red-400">{error}</div>}

            <div>
              <label className="label">Provider</label>
              <select className="input" value={provider} onChange={(e) => handleProviderChange(e.target.value)}>
                {PROVIDERS.map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
              </select>
            </div>

            <div>
              <label className="label">{isS3 ? 'S3 Endpoint' : 'Instance URL'}</label>
              <input type="url" className="input" value={providerUrl} onChange={(e) => setProviderUrl(e.target.value)} placeholder={isS3 ? 'https://minio.example.com' : (defaultUrls[provider] || 'https://...')} />
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">{isS3 ? 'The endpoint URL of your S3/MinIO service' : 'The URL of your self-hosted instance (leave blank for SaaS)'}</p>
            </div>

            <div>
              <label className="label">Display Name</label>
              <input type="text" className="input" value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="My GitLab token" />
            </div>

            <div>
              <label className="label">{isS3 ? 'Secret Key' : 'Personal Access Token'}</label>
              <div className="flex gap-2 items-end">
                <div className="flex-1">
                  <VaultInput
                    label=""
                    field="token"
                    value={token}
                    onChange={(v) => { setToken(v); setFetched(false); }}
                    vaultRef={vaultRefs.token}
                    onOpenVault={onOpenVaultPicker}
                    onRemoveVault={removeVaultRef}
                    placeholder="Paste token or pick from Vault"
                    required
                  />
                </div>
                <button
                  type="button"
                  onClick={handleFetchUserInfo}
                  disabled={fetching || (!token && !vaultRefs.token)}
                  className="btn btn-secondary whitespace-nowrap disabled:opacity-50 disabled:cursor-not-allowed mb-0"
                >
                  {fetching ? (isS3 ? 'Testing...' : 'Fetching...') : fetched ? '✓ ' + (isS3 ? 'Connected' : 'Fetched') : (isS3 ? 'Test Connection' : 'Fetch User')}
                </button>
              </div>
              <p className="text-[11px] text-[var(--text-tertiary)] mt-1">{isS3 ? 'Your S3 secret access key will be encrypted and stored securely.' : 'Your PAT will be encrypted and stored securely. Click &quot;Fetch User&quot; to auto-fill username and email.'}</p>
            </div>

            <div className={isS3 ? '' : 'grid grid-cols-2 gap-4'}>
              <div>
                <label className="label">
                  {isS3 ? 'Access Key ID' : 'Remote Username'}
                  {fetched && <span className="text-[11px] text-emerald-600 ml-1 font-normal">(auto-filled)</span>}
                </label>
                <input type="text" className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder={isS3 ? 'Your S3 access key ID' : 'For git commits'} />
              </div>
              {!isS3 && (
              <div>
                <label className="label">
                  Remote Email
                  {fetched && <span className="text-[11px] text-emerald-600 ml-1 font-normal">(auto-filled)</span>}
                </label>
                <input type="email" className="input" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="For git commits" />
              </div>
              )}
            </div>

            <label className="flex items-center gap-2">
              <input type="checkbox" checked={isDefault} onChange={(e) => setIsDefault(e.target.checked)} className="rounded" />
              <span className="text-[13px] text-[var(--text-secondary)]">Set as default for this provider</span>
            </label>
          </div>
          <div className="px-5 py-3 border-t border-[var(--border)] flex justify-end gap-2 sticky bottom-0 bg-[var(--surface)]">
            <button type="button" onClick={onClose} className="btn btn-secondary">Cancel</button>
            <button type="submit" disabled={saving} className="btn btn-primary">
              {saving ? 'Saving...' : 'Save'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
