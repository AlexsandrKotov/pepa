'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { connections, pipelineSources, type Connection, type PipelineSource } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import BrandIcon from '@/components/BrandIcon';
import JenkinsGroovyEditor from '@/components/JenkinsGroovyEditor';

interface JenkinsJob {
  name: string;
  _class: string;
  color?: string;
  lastBuild?: { number: number };
}

export default function JenkinsPanel({ enabled, sources, onOpenSource }: {
  enabled: boolean;
  sources: PipelineSource[];
  onOpenSource: (source: PipelineSource) => void;
}) {
  const [items, setItems] = useState<Connection[]>([]);
  const [connectionId, setConnectionId] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    setLoading(true);
    setError('');
    async function load() {
      try {
        const all: Connection[] = [];
        let page = 1;
        let totalPages = 1;
        do {
          const result = await connections.list({ type: 'jenkins', per_page: '100', page: String(page) });
          all.push(...(result.connections || []));
          totalPages = result.total_pages || 1;
          page++;
        } while (active && page <= totalPages);
        if (active) {
          setItems(all);
          setConnectionId(current => all.some(c => c.id === current) ? current : all[0]?.id || '');
        }
      } catch (err) {
        if (active) setError(friendlyError(err).message);
      } finally {
        if (active) setLoading(false);
      }
    }
    void load();
    return () => { active = false; };
  }, [enabled, refresh]);

  if (!enabled) return (
    <div className="card p-6 space-y-3">
      <h2 className="text-lg font-semibold">Jenkins plugin is not enabled</h2>
      <p className="text-sm text-[var(--text-secondary)]">Install Jenkins from Marketplace and enable it in Plugins to manage jobs and pipelines.</p>
      <Link href="/marketplace" className="btn btn-primary">Open Marketplace</Link>
    </div>
  );

  return (
    <div className="space-y-4">
      <div className="card overflow-hidden">
        {/* Accent bar */}
        <div className="h-1" style={{ background: 'linear-gradient(90deg, #d3923a, #ef6b3c 50%, #d3923a)' }} />

        <div className="px-4 py-3 flex flex-wrap items-center gap-x-4 gap-y-2.5">
          {/* ── Brand block ── */}
          <div className="flex items-center gap-3">
            <div className="h-10 w-10 rounded-xl flex items-center justify-center ring-1 ring-black/[0.06]" style={{ background: 'linear-gradient(145deg, rgba(211,146,58,0.10), rgba(239,107,60,0.16))' }}>
              <BrandIcon name="jenkins" size={22} />
            </div>
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-semibold text-[var(--text-primary)] leading-none">Jenkins</span>
              {connectionId ? (
                <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-emerald-600 dark:text-emerald-400 leading-none mt-0.5">
                  <span className="relative flex h-1.5 w-1.5">
                    <span className="absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75 animate-ping" />
                    <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500" />
                  </span>
                  Connected
                </span>
              ) : (
                <span className="text-[11px] text-[var(--text-tertiary)] leading-none mt-0.5">No connection</span>
              )}
            </div>
          </div>

          <div className="h-8 w-px bg-[var(--border-light)]" />

          {/* ── Connection selector ── */}
          <div className="relative flex-1 min-w-56">
            <select
              id="jenkins-connection"
              value={connectionId}
              onChange={e => setConnectionId(e.target.value)}
              disabled={loading}
              className="w-full appearance-none rounded-lg border border-[var(--border)] bg-[var(--surface)] pl-3 pr-9 py-[7px] text-sm font-medium focus:border-[var(--accent)] focus:shadow-[0_0_0_3px_rgba(0,102,255,0.08),inset_0_1px_2px_rgba(0,102,255,0.06)] outline-none transition-all"
            >
              <option value="">Select a connection</option>
              {items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
            <svg className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-tertiary)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"><path d="m6 9 6 6 6-6" /></svg>
          </div>

          {items.length > 0 && (
            <span className="inline-flex items-center gap-1.5 text-[11px] font-medium text-[var(--text-tertiary)] bg-[var(--surface-hover)] rounded-full px-2.5 py-1 leading-none">
              <span className="h-1 w-1 rounded-full bg-[var(--text-tertiary)]" />
              {items.length}
            </span>
          )}

          <div className="h-8 w-px bg-[var(--border-light)]" />

          {/* ── Action toolbar ── */}
          <div className="flex items-center rounded-lg border border-[var(--border)] bg-[var(--surface-hover)]/60 p-0.5 gap-px">
            <button className="flex items-center justify-center h-8 w-8 rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface)] transition-colors" onClick={() => setRefresh(v => v + 1)} disabled={loading} title="Refresh connections">
              <svg className={`h-[15px] w-[15px] ${loading ? 'animate-spin' : ''}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 2v6h-6" /><path d="M3 12a9 9 0 0 1 15.4-6.4L21 8" /><path d="M3 22v-6h6" /><path d="M21 12a9 9 0 0 1-15.4 6.4L3 16" /></svg>
            </button>
            <Link href="/connections" className="flex items-center justify-center h-8 w-8 rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface)] transition-colors" title="Manage connections">
              <svg className="h-[15px] w-[15px]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" /><circle cx="12" cy="12" r="3" /></svg>
            </Link>
          </div>
        </div>
      </div>
      {error && <p role="alert" className="p-3 rounded-lg bg-red-500/10 text-red-500 text-sm">{error}</p>}
      {loading ? <p role="status" className="text-sm text-[var(--text-secondary)] flex items-center gap-2"><svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 12a9 9 0 1 1-6.219-8.56" /></svg>Loading connections...</p> : !connectionId ? (
        <div className="card p-6 text-sm text-[var(--text-secondary)]">Add a Jenkins connection with its URL, username, and API token in Connections.</div>
      ) : <JenkinsJobs key={connectionId} connectionId={connectionId} sources={sources} onOpenSource={onOpenSource} />}
    </div>
  );
}

function JenkinsJobs({ connectionId, sources, onOpenSource }: {
  connectionId: string;
  sources: PipelineSource[];
  onOpenSource: (source: PipelineSource) => void;
}) {
  const [folder, setFolder] = useState('');
  const [jobs, setJobs] = useState<JenkinsJob[]>([]);
  const [selectedJob, setSelectedJob] = useState<JenkinsJob | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [refresh, setRefresh] = useState(0);
  const [start, setStart] = useState(0);
  const [saving, setSaving] = useState(false);
  const pageSize = 50;
  const fullName = selectedJob ? [folder, selectedJob.name].filter(Boolean).join('/') : '';
  const existing = sources.find(s => s.source_type === 'jenkins' && s.connection_id === connectionId && s.config?.job_name === fullName);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError('');
    connections.execute(connectionId, 'list_jobs', { folder, start, limit: pageSize })
      .then(({ data }) => { if (active) setJobs((data as { jobs?: JenkinsJob[] }).jobs || []); })
      .catch(err => { if (active) { setJobs([]); setError(friendlyError(err).message); } })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [connectionId, folder, start, refresh]);

  const openFolder = (path: string) => { setFolder(path); setStart(0); setSelectedJob(null); };
  const openRuns = async () => {
    if (existing) { onOpenSource(existing); return; }
    setSaving(true);
    setError('');
    try {
      const source = await pipelineSources.create({ name: fullName, source_type: 'jenkins', connection_id: connectionId, config: { job_name: fullName } });
      onOpenSource(source);
    } catch (err) {
      setError(friendlyError(err).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
      <div className="card p-4 space-y-3">
        <div className="flex items-center justify-between gap-2">
          <h2 className="font-semibold">Jobs and folders</h2>
          <button className="btn btn-secondary" onClick={() => setRefresh(v => v + 1)} disabled={loading}>Refresh jobs</button>
        </div>
        <div className="text-sm break-all text-[var(--text-secondary)]">
          <button onClick={() => openFolder('')} className="text-[var(--accent)]">Jenkins</button>
          {folder && <> / {folder} <button className="text-[var(--accent)] ml-2" onClick={() => openFolder(folder.split('/').slice(0, -1).join('/'))}>Up</button></>}
        </div>
        {error && <p role="alert" className="p-3 rounded-lg bg-red-500/10 text-red-500 text-sm">{error}</p>}
        {loading ? <p role="status" className="text-sm">Loading jobs...</p> : jobs.length === 0 ? <p className="text-sm text-[var(--text-secondary)]">No jobs found.</p> : (
          <div className="space-y-1">
            {jobs.map(job => {
              const isFolder = /Folder|MultiBranchProject|OrganizationFolder/.test(job._class);
              return (
                <button key={job.name} onClick={() => isFolder ? openFolder([folder, job.name].filter(Boolean).join('/')) : setSelectedJob(job)} className={`w-full text-left rounded-lg p-3 text-sm ${selectedJob?.name === job.name ? 'bg-[var(--accent-subtle)] text-[var(--accent)]' : 'hover:bg-[var(--surface-hover)]'}`}>
                  <span className="font-medium break-all">{job.name}{isFolder ? ' /' : ''}</span>
                  {job.lastBuild && <span className="block text-xs text-[var(--text-tertiary)]">Last build #{job.lastBuild.number}</span>}
                </button>
              );
            })}
          </div>
        )}
        <div className="flex items-center justify-between text-sm">
          <button disabled={loading || start === 0} onClick={() => setStart(v => Math.max(0, v - pageSize))} className="btn btn-secondary">Previous</button>
          <span>Page {start / pageSize + 1}</span>
          <button disabled={loading || jobs.length < pageSize} onClick={() => setStart(v => v + pageSize)} className="btn btn-secondary">Next</button>
        </div>
      </div>
      <div className="lg:col-span-2 card p-4 space-y-4">
        {selectedJob ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="font-semibold break-all">{fullName}</h2>
            <button onClick={openRuns} disabled={saving} className="btn btn-primary">{saving ? 'Connecting...' : existing ? 'Open runs' : 'Connect engine'}</button>
            <p className="w-full text-sm text-[var(--text-secondary)]">Open this job as an engine to run builds with parameters, sync build history, inspect stages, and read logs.</p>
          </div>
        ) : <p className="text-sm text-[var(--text-secondary)]">Select a job to manage it, or create a new pipeline.</p>}
        {(!selectedJob || selectedJob._class.includes('WorkflowJob')) && (
          <JenkinsGroovyEditor key={fullName} connectionId={connectionId} jobName={fullName} onSaved={() => setRefresh(v => v + 1)} />
        )}
      </div>
    </div>
  );
}
