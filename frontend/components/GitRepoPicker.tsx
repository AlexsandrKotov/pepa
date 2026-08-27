'use client';

import { useState, useEffect, useCallback } from 'react';
import { gitBrowser, connections as connectionsAPI, type GitGroup, type GitRepo, type Connection } from '@/lib/api';

export interface GitRepoPickerValue {
  connection_id: string;
  group_id: string;
  repo_id: string;
  repo_url: string;
  repo_full_name: string;
  branch: string;
}

interface GitRepoPickerProps {
  value: Partial<GitRepoPickerValue>;
  onChange: (value: GitRepoPickerValue) => void;
  /** Label for the picker section */
  label?: string;
  /** Available git connections to choose from */
  gitConnections?: Connection[];
  /** Whether to show the branch selector */
  showBranch?: boolean;
}

export default function GitRepoPicker({ value, onChange, label = 'Git Repository', gitConnections, showBranch = false }: GitRepoPickerProps) {
  const [connList, setConnList] = useState<Connection[]>(gitConnections || []);
  const [selectedConn, setSelectedConn] = useState(value.connection_id || '');
  const [groups, setGroups] = useState<GitGroup[]>([]);
  const [selectedGroup, setSelectedGroup] = useState(value.group_id || '');
  const [repos, setRepos] = useState<GitRepo[]>([]);
  const [selectedRepo, setSelectedRepo] = useState(value.repo_id || '');
  const [branches, setBranches] = useState<{ name: string; sha: string; protected: boolean }[]>([]);
  const [selectedBranch, setSelectedBranch] = useState(value.branch || '');

  const [loadingGroups, setLoadingGroups] = useState(false);
  const [loadingRepos, setLoadingRepos] = useState(false);
  const [loadingBranches, setLoadingBranches] = useState(false);
  const [loadingConns, setLoadingConns] = useState(!gitConnections);
  const [groupsError, setGroupsError] = useState('');

  // Load git connections if not provided
  useEffect(() => {
    if (gitConnections) {
      setConnList(gitConnections);
      setLoadingConns(false);
      return;
    }
    (async () => {
      try {
        const data = await connectionsAPI.list();
        const gitConns = (data.connections || data || []).filter(
          (c: Connection) => c.type === 'git' || c.type === 'gitlab'
        );
        setConnList(gitConns);
      } catch { /* ignore */ }
      setLoadingConns(false);
    })();
  }, [gitConnections]);

  // Sync internal state when value.connection_id changes from parent
  useEffect(() => {
    const newConnId = value.connection_id || '';
    if (newConnId && newConnId !== selectedConn) {
      setSelectedConn(newConnId);
      setSelectedGroup('');
      setSelectedRepo('');
      setSelectedBranch('');
      setGroups([]);
      setRepos([]);
      setBranches([]);
      loadGroups(newConnId);
    }
  }, [value.connection_id]); // eslint-disable-line react-hooks/exhaustive-deps

  // Auto-load groups when component mounts with a pre-selected connection
  useEffect(() => {
    if (selectedConn && groups.length === 0 && !loadingGroups) {
      loadGroups(selectedConn);
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Load groups when connection changes
  const loadGroups = useCallback(async (connId: string) => {
    if (!connId) { setGroups([]); setGroupsError(''); return; }
    setLoadingGroups(true);
    setGroupsError('');
    try {
      const data = await gitBrowser.listGroups(connId);
      setGroups(data.groups);
      if (data.groups.length === 0) {
        setGroupsError('No groups found — check token permissions');
      }
    } catch (err) {
      setGroups([]);
      setGroupsError(`Failed to load groups: ${err}`);
    }
    setLoadingGroups(false);
  }, []);

  // Load repos when group changes
  const loadRepos = useCallback(async (connId: string, groupId: string) => {
    if (!connId) { setRepos([]); return; }
    setLoadingRepos(true);
    try {
      const data = await gitBrowser.listRepos(connId, groupId || undefined);
      setRepos(data.repos);
    } catch {
      setRepos([]);
    }
    setLoadingRepos(false);
  }, []);

  // Load branches when repo changes
  const loadBranches = useCallback(async (connId: string, repoId: string) => {
    if (!connId || !repoId) { setBranches([]); return; }
    setLoadingBranches(true);
    try {
      const data = await gitBrowser.listBranches(connId, repoId);
      setBranches(data.branches);
    } catch {
      setBranches([]);
    }
    setLoadingBranches(false);
  }, []);

  // Handle connection change
  const handleConnChange = (connId: string) => {
    setSelectedConn(connId);
    setSelectedGroup('');
    setSelectedRepo('');
    setSelectedBranch('');
    setGroups([]);
    setRepos([]);
    setBranches([]);
    loadGroups(connId);
    emitChange(connId, '', '', '', '', '');
  };

  // Handle group change
  const handleGroupChange = (groupId: string) => {
    setSelectedGroup(groupId);
    setSelectedRepo('');
    setSelectedBranch('');
    setRepos([]);
    setBranches([]);
    loadRepos(selectedConn, groupId);
    emitChange(selectedConn, groupId, '', '', '', '');
  };

  // Handle repo change
  const handleRepoChange = (repoId: string) => {
    setSelectedRepo(repoId);
    setSelectedBranch('');
    setBranches([]);
    const repo = repos.find(r => r.id === repoId);
    const repoUrl = repo?.url || '';
    const repoFullName = repo?.full_name || '';
    emitChange(selectedConn, selectedGroup, repoId, repoUrl, repoFullName, '');
    if (showBranch) {
      loadBranches(selectedConn, repoId);
    }
  };

  // Handle branch change
  const handleBranchChange = (branch: string) => {
    setSelectedBranch(branch);
    const repo = repos.find(r => r.id === selectedRepo);
    emitChange(selectedConn, selectedGroup, selectedRepo, repo?.url || '', repo?.full_name || '', branch);
  };

  const emitChange = (connId: string, groupId: string, repoId: string, repoUrl: string, repoFullName: string, branch: string) => {
    onChange({ connection_id: connId, group_id: groupId, repo_id: repoId, repo_url: repoUrl, repo_full_name: repoFullName, branch });
  };

  // Detect provider kind label from connection config
  const getGroupLabel = () => {
    const conn = connList.find(c => c.id === selectedConn);
    if (!conn) return 'Group / Organization';
    const provider = (conn.config as any)?.provider || conn.type;
    switch (provider) {
      case 'github': return 'Organization';
      case 'bitbucket': return 'Workspace';
      case 'gitea': return 'Organization';
      case 'gitlab': return 'Group';
      default: return 'Group / Organization';
    }
  };

  return (
    <div className="space-y-2">
      {label && <label className="label">{label}</label>}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-2">
        {/* Connection selector */}
        <div>
          <label className="text-[11px] text-[var(--text-tertiary)]">Git Connection</label>
          {loadingConns ? (
            <div className="flex items-center gap-1.5 py-1.5">
              <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
              <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>
            </div>
          ) : (
            <select
              value={selectedConn}
              onChange={e => handleConnChange(e.target.value)}
              className="input text-[12px]"
            >
              <option value="">Select connection...</option>
              {connList.map(c => (
                <option key={c.id} value={c.id}>
                  {c.name} ({c.type === 'gitlab' ? 'GitLab' : ((c.config as any)?.provider || c.type)})
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Group/Org selector */}
        <div>
          <label className="text-[11px] text-[var(--text-tertiary)]">{getGroupLabel()}</label>
          {!selectedConn ? (
            <select disabled className="input text-[12px]" ><option>Select connection first</option></select>
          ) : loadingGroups ? (
            <div className="flex items-center gap-1.5 py-1.5">
              <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
              <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>
            </div>
          ) : (
            <>
              <select
                value={selectedGroup}
                onChange={e => handleGroupChange(e.target.value)}
                className="input text-[12px]"
              >
                <option value="">All {getGroupLabel().toLowerCase()}s</option>
                {groups.map(g => (
                  <option key={g.id} value={g.id}>{g.full_name || g.name}</option>
                ))}
              </select>
              {groupsError && <p className="text-[10px] text-red-500 mt-0.5">{groupsError}</p>}
            </>
          )}
        </div>

        {/* Repo selector */}
        <div>
          <label className="text-[11px] text-[var(--text-tertiary)]">Repository</label>
          {!selectedConn ? (
            <select disabled className="input text-[12px]"><option>Select connection first</option></select>
          ) : loadingRepos ? (
            <div className="flex items-center gap-1.5 py-1.5">
              <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
              <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>
            </div>
          ) : (
            <select
              value={selectedRepo}
              onChange={e => handleRepoChange(e.target.value)}
              className="input text-[12px]"
            >
              <option value="">Select repository...</option>
              {repos.map(r => (
                <option key={r.id} value={r.id}>
                  {r.full_name}{r.private ? ' (private)' : ''}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Branch selector (optional) */}
        {showBranch && (
          <div>
            <label className="text-[11px] text-[var(--text-tertiary)]">Branch</label>
            {!selectedRepo ? (
              <select disabled className="input text-[12px]"><option>Select repo first</option></select>
            ) : loadingBranches ? (
              <div className="flex items-center gap-1.5 py-1.5">
                <div className="animate-spin rounded-full h-3 w-3 border-b-2 border-[var(--accent)]" />
                <span className="text-[11px] text-[var(--text-tertiary)]">Loading...</span>
              </div>
            ) : (
              <select
                value={selectedBranch}
                onChange={e => handleBranchChange(e.target.value)}
                className="input text-[12px]"
              >
                <option value="">Select branch...</option>
                {branches.map(b => (
                  <option key={b.name} value={b.name}>
                    {b.name}{b.protected ? ' (protected)' : ''}
                  </option>
                ))}
              </select>
            )}
          </div>
        )}
      </div>

      {/* Selected summary */}
      {selectedRepo && (
        <p className="text-[11px] text-green-600 flex items-center gap-1">
          Selected: <span className="font-medium font-mono">{repos.find(r => r.id === selectedRepo)?.full_name || selectedRepo}</span>
          {selectedBranch && <span> @ {selectedBranch}</span>}
        </p>
      )}
    </div>
  );
}
