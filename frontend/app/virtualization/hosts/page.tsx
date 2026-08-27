'use client';

import { useState, useEffect } from 'react';
import { connections as connectionsAPI, virtualization, type Connection } from '@/lib/api';
import Link from 'next/link';

export default function ProxmoxHostsPage() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [testing, setTesting] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<Record<string, { status: string; message: string }>>({});

  useEffect(() => { loadConnections(); }, []);

  const loadConnections = async () => {
    try {
      const res = await connectionsAPI.list();
      setConnections((res.connections || []).filter(c => c.type === 'proxmox'));
    } catch { /* ignore */ }
    setLoading(false);
  };

  const handleTest = async (id: string) => {
    setTesting(id);
    try {
      const result = await virtualization.proxmox.testConnection();
      const warning = result.data?.warning;
      setTestResult(prev => ({
        ...prev,
        [id]: warning
          ? { status: 'error', message: warning }
          : { status: 'connected', message: 'Connection successful' },
      }));
    } catch (err) {
      setTestResult(prev => ({ ...prev, [id]: { status: 'error', message: err instanceof Error ? err.message : 'Test failed' } }));
    }
    setTesting(null);
  };

  const statusColor = (s: string) => {
    switch (s) {
      case 'connected': return 'bg-emerald-500/10 text-emerald-600';
      case 'error': return 'bg-red-500/10 text-red-500';
      default: return 'bg-[var(--border-light)] text-[var(--text-tertiary)]';
    }
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Proxmox Hosts</h1>
            <p className="page-subtitle-modern">Manage Proxmox VE connections and credentials</p>
          </div>
          <Link href="/connections?type=proxmox" className="btn btn-primary">+ Add Proxmox Host</Link>
        </div>

        {loading ? (
          <div className="card card-body text-center py-16">
            <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
              <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
              <span className="text-[13px]">Loading...</span>
            </div>
          </div>
        ) : connections.length === 0 ? (
          <div className="card card-body text-center py-16">
            <div className="text-5xl mb-4 opacity-20">🖥</div>
            <p className="text-[14px] text-[var(--text-secondary)] mb-1">No Proxmox connections configured</p>
            <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
              Add a Proxmox VE connection to start managing virtual machines and containers
            </p>
            <Link href="/connections?type=proxmox" className="btn btn-primary">+ Add First Connection</Link>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {connections.map(conn => {
              const tr = testResult[conn.id];
              return (
                <div key={conn.id} className="card p-5 modern-card-hover group" style={{ borderRadius: '12px' }}>
                  <div className="flex items-start justify-between mb-3">
                    <div className="flex items-center gap-2">
                      <span className="text-xl">🖥</span>
                      <div>
                        <h3 className="text-[13px] font-semibold text-[var(--text-primary)]">{conn.name}</h3>
                        <span className="text-[10px] text-[var(--text-tertiary)]">Proxmox VE</span>
                      </div>
                    </div>
                    <span className={`text-[10px] px-2 py-0.5 rounded-full font-medium ${statusColor(conn.status)}`}>
                      {conn.status}
                    </span>
                  </div>

                  {conn.description && (
                    <p className="text-[12px] text-[var(--text-secondary)] mb-3 line-clamp-2">{conn.description}</p>
                  )}

                  <div className="space-y-1.5 mb-4">
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-[var(--text-tertiary)] w-14">URL:</span>
                      <span className="font-mono text-[var(--text-secondary)] truncate">{String(conn.config?.url || '-')}</span>
                    </div>
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-[var(--text-tertiary)] w-14">Token:</span>
                      <span className="font-mono text-[var(--text-secondary)] truncate">{String(conn.config?.token_id || '-')}</span>
                    </div>
                  </div>

                  {tr && (
                    <div className={`mb-3 p-2 rounded-lg text-[11px] ${tr.status === 'connected' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>
                      {tr.message}
                    </div>
                  )}

                  <div className="flex gap-2 pt-3 border-t border-[var(--border-light)]">
                    <button
                      onClick={() => handleTest(conn.id)}
                      disabled={testing === conn.id}
                      className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors disabled:opacity-50"
                    >
                      {testing === conn.id ? 'Testing...' : 'Test Connection'}
                    </button>
                    <Link href={`/connections?id=${conn.id}`} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded-lg transition-colors">
                      Edit
                    </Link>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* Info box */}
        <div className="card card-body">
          <h3 className="text-[13px] font-semibold text-[var(--text-primary)] mb-2">How to configure Proxmox</h3>
          <div className="text-[12px] text-[var(--text-secondary)] space-y-1">
            <p>1. Go to Proxmox VE web interface → Datacenter → Permissions → API Tokens</p>
            <p>2. Create a new API Token for a user (uncheck "Privilege Separation" for full access)</p>
            <p>3. Copy the Token ID (format: user@realm!tokenname) and Token Secret</p>
            <p>4. Add a new Proxmox connection in the Connections page with these credentials</p>
          </div>
        </div>
      </div>
    </div>
  );
}
