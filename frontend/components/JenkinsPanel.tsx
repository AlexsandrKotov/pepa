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
      <div className="card p-4 flex flex-wrap items-center gap-3">
        <BrandIcon name="jenkins" size={28} />
        <label htmlFor="jenkins-connection" className="text-sm font-medium">Jenkins connection</label>
        <select id="jenkins-connection" value={connectionId} onChange={e => setConnectionId(e.target.value)} disabled={loading} className="flex-1 min-w-48 rounded-lg border border-[var(--border)] bg-[var(--surface)] px-3 py-2 text-sm">
          <option value="">Select a connection</option>
          {items.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <button className="btn btn-secondary" onClick={() => setRefresh(v => v + 1)} disabled={loading}>Refresh connections</button>
        <Link href="/connections" className="btn btn-secondary">Manage connections</Link>
      </div>
      {error && <p role="alert" className="p-3 rounded-lg bg-red-500/10 text-red-500">{error}</p>}
      {loading ? <p role="status">Loading Jenkins connections...</p> : !connectionId ? (
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
