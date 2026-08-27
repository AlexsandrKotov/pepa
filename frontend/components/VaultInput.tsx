'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import { vault as vaultAPI, type VaultPath } from '@/lib/api';

// ── Vault Input Field ──────────────────────────────────────────────────────
// A wrapper input that lets users pick a value from Vault instead of typing it.
// Shows a lock icon button that opens the Vault picker modal.

export function VaultInput({ label, field, value, onChange, vaultRef, onOpenVault, onRemoveVault, placeholder, isTextarea, required }: {
  label: string;
  field: string;
  value: string;
  onChange: (value: string) => void;
  vaultRef?: string;
  onOpenVault: (field: string) => void;
  onRemoveVault?: (field: string) => void;
  placeholder?: string;
  isTextarea?: boolean;
  required?: boolean;
}) {
  const isVault = !!vaultRef;
  const inputClass = "w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent pr-10";

  return (
    <div>
      <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">
        {label}
        {isVault && <span className="ml-1.5 text-[10px] px-1.5 py-0.5 rounded bg-violet-500/10 text-violet-500 font-medium">VAULT</span>}
        {required && <span className="text-red-500 ml-0.5">*</span>}
      </label>
      <div className="relative">
        {isTextarea ? (
          <textarea
            value={isVault ? vaultRef! : value}
            onChange={e => onChange(e.target.value)}
            rows={3}
            className={`${inputClass} font-mono text-xs`}
            placeholder={isVault ? vaultRef : placeholder}
            readOnly={isVault}
          />
        ) : (
          <input
            type={field.includes('password') || field.includes('token') || field.includes('key') || field.includes('secret') ? 'password' : 'text'}
            value={isVault ? vaultRef! : value}
            onChange={e => onChange(e.target.value)}
            className={inputClass}
            placeholder={isVault ? vaultRef : placeholder}
            readOnly={isVault}
          />
        )}
        <button
          type="button"
          onClick={() => onOpenVault(field)}
          className={`absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded transition-colors ${
            isVault ? 'text-violet-500 hover:bg-violet-500/10' : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--bg)]'
          }`}
          title={isVault ? 'Change Vault reference' : 'Pick from Vault'}
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
          </svg>
        </button>
      </div>
      {isVault && (
        <button
          type="button"
          onClick={() => { onChange(''); onRemoveVault?.(field); }}
          className="text-[11px] text-red-500 hover:underline mt-1"
        >
          Remove Vault reference
        </button>
      )}
    </div>
  );
}

// ── Vault Picker Modal ─────────────────────────────────────────────────────
// Modal for browsing Vault secrets and selecting a specific key.

export function VaultPickerModal({ onSelect, onClose }: {
  onSelect: (ref: string) => void;
  onClose: () => void;
}) {
  useEscapeKey(onClose);
  const [paths, setPaths] = useState<VaultPath[]>([]);
  const [prefix, setPrefix] = useState('');
  const [loading, setLoading] = useState(true);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [secretData, setSecretData] = useState<Record<string, string> | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  const loadPaths = async (p: string) => {
    setLoading(true);
    try {
      const res = await vaultAPI.paths(p || undefined);
      setPaths(res.paths || []);
    } catch { setPaths([]); }
    setLoading(false);
  };

  useEffect(() => { loadPaths(''); }, []);

  const handleSelectPath = async (path: string, hasChildren: boolean) => {
    if (hasChildren) {
      setPrefix(path);
      loadPaths(path);
      setSelectedPath(null);
      setSecretData(null);
      setSelectedKey(null);
    } else {
      setSelectedPath(path);
      try {
        const res = await vaultAPI.getSecret(path);
        setSecretData(res.secret.data);
        const keys = Object.keys(res.secret.data);
        if (keys.length === 1) setSelectedKey(keys[0]);
      } catch { setSecretData(null); }
    }
  };

  const handleConfirm = () => {
    if (selectedPath && selectedKey) {
      onSelect(`vault:${selectedPath}/${selectedKey}`);
    }
  };

  const breadcrumbParts = prefix ? prefix.split('/').filter(Boolean) : [];
  const filteredPaths = search
    ? paths.filter(p => p.path.toLowerCase().includes(search.toLowerCase()))
    : paths;

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-[60]" onClick={onClose}>
      <div className="bg-[var(--surface)] rounded-2xl p-6 max-w-lg w-full mx-4 max-h-[80vh] overflow-hidden flex flex-col" onClick={e => e.stopPropagation()}>
        <div className="flex items-center gap-3 mb-4">
          <span className="text-xl">&#x1F510;</span>
          <div>
            <h2 className="text-lg font-bold text-[var(--text-primary)]">Pick from Vault</h2>
            <p className="text-xs text-[var(--text-tertiary)]">Browse your Vault secrets and select a value</p>
          </div>
          <button onClick={onClose} className="ml-auto text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
        </div>

        {/* Breadcrumb */}
        <div className="flex items-center gap-1 text-xs mb-3 flex-wrap">
          <button onClick={() => { setPrefix(''); loadPaths(''); setSecretData(null); setSelectedPath(null); }} className="text-blue-600 hover:underline">secret/</button>
          {breadcrumbParts.map((part, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="text-[var(--text-tertiary)]">/</span>
              <button
                onClick={() => { const np = breadcrumbParts.slice(0, i + 1).join('/'); setPrefix(np); loadPaths(np); setSecretData(null); setSelectedPath(null); }}
                className="text-blue-600 hover:underline"
              >
                {part}
              </button>
            </span>
          ))}
        </div>

        {/* Search */}
        <input
          type="text"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Filter secrets..."
          className="w-full px-3 py-1.5 border border-[var(--border)] rounded-lg text-sm mb-3 focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
        />

        {/* Paths list */}
        <div className="flex-1 overflow-y-auto border border-[var(--border)] rounded-lg mb-3">
          {loading ? (
            <div className="p-6 text-center text-sm text-[var(--text-tertiary)]">Loading...</div>
          ) : filteredPaths.length === 0 ? (
            <div className="p-6 text-center text-sm text-[var(--text-tertiary)]">
              {search ? 'No matching secrets' : 'No secrets at this path'}
            </div>
          ) : (
            <div className="divide-y divide-[var(--border-light)]">
              {filteredPaths.map(p => (
                <button
                  key={p.path}
                  onClick={() => handleSelectPath(p.path, p.has_children)}
                  className={`w-full flex items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-blue-500/10 ${
                    selectedPath === p.path ? 'bg-[var(--accent-subtle)] text-[var(--accent)]' : 'text-[var(--text-primary)]'
                  }`}
                >
                  <span>{p.has_children ? '\u{1F4C1}' : '\u{1F511}'}</span>
                  <span className="truncate font-mono text-xs">{p.path.split('/').pop()}</span>
                  {selectedPath === p.path && <span className="ml-auto text-[var(--accent)] text-xs">&check;</span>}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Key selector */}
        {secretData && (
          <div className="mb-3">
            <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">Select key:</label>
            <div className="flex flex-wrap gap-1.5">
              {Object.keys(secretData).map(key => (
                <button
                  key={key}
                  onClick={() => setSelectedKey(key)}
                  className={`px-2.5 py-1 rounded-lg text-xs font-mono transition-colors ${
                    selectedKey === key
                      ? 'bg-violet-500/10 text-violet-500 border border-violet-500/20'
                      : 'bg-[var(--border-light)] text-[var(--text-tertiary)] hover:bg-[var(--border)] border border-transparent'
                  }`}
                >
                  {key}
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Actions */}
        <div className="flex gap-2 pt-3 border-t border-[var(--border)]">
          <button
            onClick={handleConfirm}
            disabled={!selectedPath || !selectedKey}
            className="flex-1 py-2 bg-violet-600 text-white rounded-lg hover:bg-violet-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors text-sm font-medium"
          >
            {selectedPath && selectedKey ? `Use vault:${selectedPath}/${selectedKey}` : 'Select a secret'}
          </button>
          <button onClick={onClose} className="px-4 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--border-light)] transition-colors text-sm">
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

// ── Hook for managing Vault refs in a form ─────────────────────────────────
// Provides state and handlers for Vault references in any form.

export function useVaultPicker() {
  const [vaultPickerTarget, setVaultPickerTarget] = useState<{ field: string } | null>(null);
  const [vaultRefs, setVaultRefs] = useState<Record<string, string>>({});

  const onOpenVaultPicker = (field: string) => setVaultPickerTarget({ field });

  const handleVaultSelect = (ref: string) => {
    if (vaultPickerTarget) {
      setVaultRefs(prev => ({ ...prev, [vaultPickerTarget.field]: ref }));
    }
    setVaultPickerTarget(null);
  };

  const VaultPicker = vaultPickerTarget ? (
    <VaultPickerModal
      onSelect={handleVaultSelect}
      onClose={() => setVaultPickerTarget(null)}
    />
  ) : null;

  const removeVaultRef = (field: string) => {
    setVaultRefs(prev => {
      const next = { ...prev };
      delete next[field];
      return next;
    });
  };

  return { vaultRefs, setVaultRefs, onOpenVaultPicker, VaultPicker, removeVaultRef };
}
