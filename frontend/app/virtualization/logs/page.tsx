'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  virtualization,
  type ProxmoxNode,
  type ProxmoxTask,
  type ProxmoxSyslogLine,
} from '@/lib/api';
import Link from 'next/link';

type Tab = 'tasks' | 'syslog';

export default function LogsPage() {
  const [nodes, setNodes] = useState<ProxmoxNode[]>([]);
  const [node, setNode] = useState('');
  const [tab, setTab] = useState<Tab>('tasks');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  // Tasks state
  const [tasks, setTasks] = useState<ProxmoxTask[]>([]);
  const [selectedTask, setSelectedTask] = useState<ProxmoxTask | null>(null);
  const [taskLog, setTaskLog] = useState<ProxmoxSyslogLine[]>([]);
  const [taskLogLoading, setTaskLogLoading] = useState(false);

  // Syslog state
  const [syslog, setSyslog] = useState<ProxmoxSyslogLine[]>([]);
  const [syslogLimit, setSyslogLimit] = useState(100);

  useEffect(() => {
    virtualization.proxmox.listNodes()
      .then(res => {
        const list = Array.isArray(res.data) ? res.data : [];
        setNodes(list);
        const first = list.find(n => n.status === 'online')?.node || '';
        setNode(first);
      })
      .catch(() => setError('Failed to load nodes'));
  }, []);

  const loadTasks = useCallback(async () => {
    if (!node) return;
    setLoading(true);
    setError('');
    try {
      const res = await virtualization.proxmox.nodeTasks(node, 50);
      setTasks(Array.isArray(res.data) ? res.data : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load tasks');
    }
    setLoading(false);
  }, [node]);

  const loadSyslog = useCallback(async () => {
    if (!node) return;
    setLoading(true);
    setError('');
    try {
      const res = await virtualization.proxmox.nodeSyslog(node, syslogLimit);
      setSyslog(Array.isArray(res.data) ? res.data : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load syslog');
    }
    setLoading(false);
  }, [node, syslogLimit]);

  useEffect(() => {
    setSelectedTask(null);
    setTaskLog([]);
    if (tab === 'tasks') loadTasks();
    else loadSyslog();
  }, [tab, loadTasks, loadSyslog]);

  const openTaskLog = async (task: ProxmoxTask) => {
    if (selectedTask?.upid === task.upid) {
      setSelectedTask(null);
      return;
    }
    setSelectedTask(task);
    setTaskLog([]);
    setTaskLogLoading(true);
    try {
      const res = await virtualization.proxmox.taskLog(task.node, task.upid);
      setTaskLog(Array.isArray(res.data) ? res.data : []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load task log');
    }
    setTaskLogLoading(false);
  };

  const formatTime = (ts?: number) =>
    ts ? new Date(ts * 1000).toLocaleString() : '-';

  const taskStatusBadge = (task: ProxmoxTask) => {
    if (!task.status) return <span className="px-2 py-0.5 rounded-full text-[10px] bg-yellow-500/10 text-yellow-600">running</span>;
    const ok = task.status === 'OK' && !task.iserr;
    return (
      <span className={`px-2 py-0.5 rounded-full text-[10px] font-medium ${ok ? 'bg-emerald-500/10 text-emerald-600' : 'bg-red-500/10 text-red-500'}`}>
        {task.exitstatus || task.status}
      </span>
    );
  };

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Logs & Tasks</h1>
            <p className="page-subtitle-modern">Node syslog and operation history from Proxmox</p>
          </div>
          <Link href="/virtualization" className="btn btn-secondary">Back to Dashboard</Link>
        </div>

        {/* Controls */}
        <div className="card card-body flex flex-wrap items-center gap-4">
          <div className="flex items-center gap-2">
            <label className="text-[11px] font-medium text-[var(--text-secondary)]">Node</label>
            <select
              value={node}
              onChange={e => setNode(e.target.value)}
              className="px-3 py-1.5 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)]"
            >
              {nodes.map(n => <option key={n.node} value={n.node}>{n.node}</option>)}
            </select>
          </div>

          <div className="flex gap-1 p-1 bg-[var(--border-light)]/50 rounded-lg">
            {(['tasks', 'syslog'] as Tab[]).map(t => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`px-3 py-1.5 text-[11px] rounded-md transition-colors ${
                  tab === t
                    ? 'bg-[var(--surface)] text-[var(--text-primary)] font-medium shadow-sm'
                    : 'text-[var(--text-tertiary)] hover:text-[var(--text-secondary)]'
                }`}
              >
                {t === 'tasks' ? 'Task History' : 'Syslog'}
              </button>
            ))}
          </div>

          {tab === 'syslog' && (
            <div className="flex items-center gap-2">
              <label className="text-[11px] font-medium text-[var(--text-secondary)]">Lines</label>
              <select
                value={syslogLimit}
                onChange={e => setSyslogLimit(parseInt(e.target.value, 10))}
                className="px-3 py-1.5 text-[12px] border border-[var(--border)] rounded-lg bg-[var(--surface)] text-[var(--text-primary)]"
              >
                {[50, 100, 250, 500].map(v => <option key={v} value={v}>{v}</option>)}
              </select>
            </div>
          )}

          <button
            onClick={() => (tab === 'tasks' ? loadTasks() : loadSyslog())}
            disabled={loading}
            className="btn btn-secondary ml-auto"
          >
            {loading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 rounded-xl p-3 text-[12px] text-red-500 flex items-center justify-between">
            <span>{error}</span>
            <button onClick={() => setError('')} className="text-xs">Dismiss</button>
          </div>
        )}

        {tab === 'tasks' ? (
          <div className="space-y-4">
            <div className="card overflow-hidden">
              {tasks.length === 0 ? (
                <div className="card-body text-center py-12">
                  <p className="text-[12px] text-[var(--text-tertiary)]">No tasks recorded on this node</p>
                </div>
              ) : (
                <table className="w-full text-[12px]">
                  <thead>
                    <tr className="border-b border-[var(--border)]">
                      <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Start time</th>
                      <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">End time</th>
                      <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">User</th>
                      <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Type</th>
                      <th className="text-left px-4 py-3 font-medium text-[var(--text-tertiary)]">Status</th>
                    </tr>
                  </thead>
                  <tbody>
                    {tasks.map(task => (
                      <tr
                        key={task.upid}
                        onClick={() => openTaskLog(task)}
                        className={`border-b border-[var(--border-light)] cursor-pointer transition-colors ${
                          selectedTask?.upid === task.upid ? 'bg-[var(--border-light)]/60' : 'hover:bg-[var(--border-light)]/30'
                        }`}
                      >
                        <td className="px-4 py-2.5 text-[var(--text-secondary)]">{formatTime(task.starttime)}</td>
                        <td className="px-4 py-2.5 text-[var(--text-secondary)]">{formatTime(task.endtime)}</td>
                        <td className="px-4 py-2.5 text-[var(--text-secondary)]">{task.user}</td>
                        <td className="px-4 py-2.5 font-mono text-[var(--text-primary)]">{task.type}</td>
                        <td className="px-4 py-2.5">{taskStatusBadge(task)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>

            {selectedTask && (
              <div className="card">
                <div className="card-header flex items-center justify-between border-b border-[var(--border-light)] px-4 py-3">
                  <p className="text-[12px] font-medium text-[var(--text-primary)]">
                    Task log — <span className="font-mono">{selectedTask.type}</span> ({selectedTask.upid})
                  </p>
                  <button onClick={() => setSelectedTask(null)} className="text-[11px] text-[var(--accent)] hover:underline">Close</button>
                </div>
                <pre className="bg-[#1e1e1e] text-[#d4d4d4] p-4 rounded-b-lg text-[11px] font-mono overflow-auto max-h-[400px] whitespace-pre-wrap">
                  {taskLogLoading
                    ? 'Loading task log...'
                    : taskLog.length > 0
                      ? taskLog.map(l => l.t).join('\n')
                      : 'No log output for this task'}
                </pre>
              </div>
            )}
          </div>
        ) : (
          <div className="card">
            <div className="card-header border-b border-[var(--border-light)] px-4 py-3">
              <p className="text-[12px] font-medium text-[var(--text-primary)]">Syslog — {node}</p>
            </div>
            <pre className="bg-[#1e1e1e] text-[#d4d4d4] p-4 rounded-b-lg text-[11px] font-mono overflow-auto max-h-[600px] whitespace-pre-wrap">
              {loading
                ? 'Loading syslog...'
                : syslog.length > 0
                  ? syslog.map(l => l.t).join('\n')
                  : 'No syslog entries'}
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}
