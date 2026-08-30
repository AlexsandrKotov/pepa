'use client';

import { useState, useEffect } from 'react';
import { useEscapeKey } from '@/hooks/useEscapeKey';
import Link from 'next/link';
import { blueprintGroups as blueprintGroupsAPI, blueprints as blueprintsAPI, type BlueprintGroup, type ServiceBlueprint } from '@/lib/api';

const sourceIcons: Record<string, string> = {
  container: '🐳', helm_git: '🔀', helm_http: '🌐', helm_oci: '📦', docker_compose: '🐙',
};

export default function BlueprintGroupsPage() {
  const [groups, setGroups] = useState<BlueprintGroup[]>([]);
  const [allBlueprints, setAllBlueprints] = useState<ServiceBlueprint[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState<BlueprintGroup | null>(null);
  const [form, setForm] = useState({ name: '', description: '' });
  const [showAddBP, setShowAddBP] = useState<string | null>(null); // group ID for adding blueprints
  const [selectedBPIds, setSelectedBPIds] = useState<Set<string>>(new Set());
  const [dragGroupIdx, setDragGroupIdx] = useState<number | null>(null);
  const [dragOverGroupIdx, setDragOverGroupIdx] = useState<number | null>(null);
  const [deploying, setDeploying] = useState<string | null>(null);
  const [deployLog, setDeployLog] = useState<{ groupId: string; lines: string[] } | null>(null);

  useEscapeKey(() => {
    if (showForm) { setShowForm(false); setEditing(null); }
    if (showAddBP) setShowAddBP(null);
  }, showForm || !!showAddBP);

  const load = () => {
    blueprintGroupsAPI.list().then(res => setGroups(res.groups || [])).catch(() => {});
    blueprintsAPI.list().then(res => setAllBlueprints(res.blueprints || [])).catch(() => {});
  };

  useEffect(() => { load(); }, []);

  const openCreate = () => {
    setEditing(null);
    setForm({ name: '', description: '' });
    setShowForm(true);
  };

  const openEdit = (g: BlueprintGroup) => {
    setEditing(g);
    setForm({ name: g.name, description: g.description });
    setShowForm(true);
  };

  const handleSave = async () => {
    if (!form.name.trim()) return;
    try {
      if (editing) {
        await blueprintGroupsAPI.update(editing.id, { name: form.name.trim(), description: form.description.trim() });
      } else {
        await blueprintGroupsAPI.create({ name: form.name.trim(), description: form.description.trim() });
      }
      setShowForm(false);
      load();
    } catch (err) {
      console.error('Failed to save group:', err);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this group? Blueprints will become ungrouped.')) return;
    try {
      await blueprintGroupsAPI.delete(id);
      load();
    } catch (err) {
      console.error('Failed to delete group:', err);
    }
  };

  const handleAddBlueprints = async (groupId: string) => {
    if (selectedBPIds.size === 0) return;
    try {
      for (const bpId of selectedBPIds) {
        const bp = allBlueprints.find(b => b.id === bpId);
        if (bp) {
          await blueprintsAPI.update(bpId, { ...bp, group_id: groupId });
        }
      }
      setShowAddBP(null);
      setSelectedBPIds(new Set());
      load();
    } catch (err) {
      console.error('Failed to add blueprints:', err);
    }
  };

  const handleRemoveBlueprint = async (bpId: string) => {
    try {
      const bp = allBlueprints.find(b => b.id === bpId);
      if (bp) {
        await blueprintsAPI.update(bpId, { ...bp, group_id: null, group_position: 0 });
      }
      load();
    } catch (err) {
      console.error('Failed to remove blueprint:', err);
    }
  };

  // Drag & drop for groups
  const handleGroupDragStart = (idx: number) => setDragGroupIdx(idx);
  const handleGroupDragOver = (e: React.DragEvent, idx: number) => {
    e.preventDefault();
    setDragOverGroupIdx(idx);
  };
  const handleGroupDrop = async (idx: number) => {
    if (dragGroupIdx === null || dragGroupIdx === idx) { setDragGroupIdx(null); setDragOverGroupIdx(null); return; }
    // We just reorder visually; backend doesn't have a group reorder endpoint yet
    const next = [...groups];
    const [moved] = next.splice(dragGroupIdx, 1);
    next.splice(idx, 0, moved);
    setGroups(next);
    setDragGroupIdx(null);
    setDragOverGroupIdx(null);
  };

  // Drag & drop for blueprints within a group
  const [dragBPIdx, setDragBPIdx] = useState<number | null>(null);
  const [dragOverBPIdx, setDragOverBPIdx] = useState<number | null>(null);
  const [dragBPGroupId, setDragBPGroupId] = useState<string | null>(null);

  const handleBPDragStart = (groupId: string, idx: number) => {
    setDragBPGroupId(groupId);
    setDragBPIdx(idx);
  };
  const handleBPDragOver = (e: React.DragEvent, idx: number) => {
    e.preventDefault();
    setDragOverBPIdx(idx);
  };
  const handleBPDrop = async (groupId: string, idx: number) => {
    if (dragBPIdx === null || dragBPIdx === idx || dragBPGroupId !== groupId) {
      setDragBPIdx(null); setDragOverBPIdx(null); return;
    }
    const group = groups.find(g => g.id === groupId);
    if (!group) return;
    const bps = [...group.blueprints];
    const [moved] = bps.splice(dragBPIdx, 1);
    bps.splice(idx, 0, moved);
    // Update locally
    setGroups(groups.map(g => g.id === groupId ? { ...g, blueprints: bps } : g));
    // Persist order
    await blueprintGroupsAPI.reorder(groupId, bps.map(b => b.id));
    setDragBPIdx(null);
    setDragOverBPIdx(null);
  };

  const handleDeployGroup = async (group: BlueprintGroup, target: 'kubernetes' | 'docker') => {
    setDeploying(group.id);
    const lines: string[] = [];
    setDeployLog({ groupId: group.id, lines });

    const addLog = (msg: string) => {
      lines.push(`[${new Date().toLocaleTimeString()}] ${msg}`);
      setDeployLog({ groupId: group.id, lines: [...lines] });
    };

    try {
      if (target === 'docker') {
        // For Docker deploy, we'd need a docker host selector - for now just log
        addLog('Docker deploy: select a Docker host target (use Docker Services page for individual deploys)');
      } else {
        addLog(`Deploying ${group.blueprints.length} blueprint(s) from "${group.name}" to Kubernetes...`);
        // Use the group deploy endpoint - but we need a cluster ID
        // For now, navigate to pipeline builder with the group's blueprints
        addLog('Use Pipeline Builder for Kubernetes deploys with cluster selection');
      }
    } catch (err) {
      addLog(`Error: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
    setDeploying(null);
  };

  // Available blueprints (not in any group)
  const ungroupedBlueprints = allBlueprints.filter(bp => !bp.group_id);
  const availableForGroup = (groupId: string) =>
    ungroupedBlueprints.filter(bp => !groups.some(g => g.id !== groupId && g.blueprints.some(b => b.id === bp.id)));

  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">
        {/* Header */}
        <div className="page-animate flex items-center justify-between">
          <div>
            <h1 className="page-title-modern">Blueprint Groups</h1>
            <p className="page-subtitle-modern">Organize blueprints into deployable groups with custom ordering</p>
          </div>
          <div className="flex gap-2">
            <Link href="/pipeline-blueprints" className="btn btn-secondary text-[12px]">
              ← All Blueprints
            </Link>
            <button onClick={openCreate} className="btn btn-primary">
              + New Group
            </button>
          </div>
        </div>

        {/* Info */}
        <div className="bg-blue-500/10 border border-blue-500/20 rounded-xl p-4 page-animate-up page-delay-1">
          <p className="text-[13px] text-blue-500">
            <span className="font-medium">Groups</span> let you organize blueprints into named collections.
            Drag blueprints within a group to set deploy order. Deploy an entire group to a target in one action.
          </p>
        </div>

        {/* Groups List */}
        {groups.length === 0 ? (
          <div className="card card-body text-center py-16">
            <div className="text-5xl mb-4 opacity-20">📂</div>
            <p className="text-[14px] text-[var(--text-secondary)] mb-1">No groups yet</p>
            <p className="text-[12px] text-[var(--text-tertiary)] mb-5">
              Create groups to organize your blueprints for ordered deployment
            </p>
            <button onClick={openCreate} className="btn btn-primary">+ Create First Group</button>
          </div>
        ) : (
          <div className="space-y-4 page-animate-up page-delay-2">
            {groups.map((group, gIdx) => (
              <div
                key={group.id}
                draggable
                onDragStart={() => handleGroupDragStart(gIdx)}
                onDragOver={(e) => handleGroupDragOver(e, gIdx)}
                onDrop={() => handleGroupDrop(gIdx)}
                onDragEnd={() => { setDragGroupIdx(null); setDragOverGroupIdx(null); }}
                className={`card p-5 transition-all ${
                  dragOverGroupIdx === gIdx && dragGroupIdx !== gIdx
                    ? 'border-[var(--accent)] border-dashed bg-[var(--accent-subtle)]'
                    : dragGroupIdx === gIdx
                      ? 'opacity-40'
                      : 'hover:border-[var(--text-tertiary)]'
                }`}
                style={{ borderRadius: '12px' }}
              >
                {/* Group header */}
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-3">
                    <div className="cursor-grab active:cursor-grabbing text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M4 8h16M4 16h16" />
                      </svg>
                    </div>
                    <div>
                      <h3 className="text-[14px] font-semibold text-[var(--text-primary)]">{group.name}</h3>
                      {group.description && (
                        <p className="text-[11px] text-[var(--text-tertiary)]">{group.description}</p>
                      )}
                    </div>
                    <span className="text-[10px] px-2 py-0.5 bg-[var(--border-light)] text-[var(--text-secondary)] rounded-full">
                      {group.blueprints.length} blueprint(s)
                    </span>
                  </div>
                  <div className="flex gap-1.5">
                    <button onClick={() => setShowAddBP(group.id)} className="text-[11px] px-2.5 py-1 text-[var(--accent)] hover:bg-[var(--accent-subtle)] rounded-lg transition-colors">
                      + Add Blueprints
                    </button>
                    <button onClick={() => openEdit(group)} className="text-[11px] px-2.5 py-1 text-[var(--text-tertiary)] hover:bg-[var(--border-light)] rounded-lg transition-colors">
                      Edit
                    </button>
                    <button onClick={() => handleDelete(group.id)} className="text-[11px] px-2.5 py-1 text-red-500 hover:bg-red-500/10 rounded-lg transition-colors">
                      Delete
                    </button>
                  </div>
                </div>

                {/* Member blueprints */}
                {group.blueprints.length === 0 ? (
                  <div className="text-center py-6 border border-dashed border-[var(--border)] rounded-lg">
                    <p className="text-[12px] text-[var(--text-tertiary)]">
                      No blueprints in this group. <button onClick={() => setShowAddBP(group.id)} className="text-[var(--accent)] hover:underline">Add some</button>
                    </p>
                  </div>
                ) : (
                  <div className="space-y-1.5">
                    <p className="text-[10px] text-[var(--text-tertiary)] uppercase tracking-wider mb-1">Deploy order (drag to reorder)</p>
                    {group.blueprints.map((bp, bpIdx) => (
                      <div
                        key={bp.id}
                        draggable
                        onDragStart={() => handleBPDragStart(group.id, bpIdx)}
                        onDragOver={(e) => handleBPDragOver(e, bpIdx)}
                        onDrop={() => handleBPDrop(group.id, bpIdx)}
                        onDragEnd={() => { setDragBPIdx(null); setDragOverBPIdx(null); }}
                        className={`flex items-center gap-3 p-2.5 rounded-lg border transition-all ${
                          dragOverBPIdx === bpIdx && dragBPIdx !== bpIdx
                            ? 'border-[var(--accent)] border-dashed bg-[var(--accent-subtle)]'
                            : dragBPIdx === bpIdx
                              ? 'opacity-40 border-[var(--border)]'
                              : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <div className="cursor-grab active:cursor-grabbing text-[var(--text-tertiary)]">
                          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M4 8h16M4 16h16" />
                          </svg>
                        </div>
                        <span className="text-[11px] text-[var(--text-tertiary)] w-5 text-right font-mono">{bpIdx + 1}</span>
                        <span className="text-sm">{sourceIcons[bp.source_type] || '📦'}</span>
                        <div className="flex-1 min-w-0">
                          <span className="text-[12px] font-medium text-[var(--text-primary)]">{bp.name}</span>
                          <span className="text-[10px] text-[var(--text-tertiary)] ml-2">{bp.source_type}</span>
                        </div>
                        <button
                          onClick={() => handleRemoveBlueprint(bp.id)}
                          className="text-[10px] text-[var(--text-tertiary)] hover:text-red-500 transition-colors"
                          title="Remove from group"
                        >
                          ✕
                        </button>
                      </div>
                    ))}
                  </div>
                )}

                {/* Deploy log */}
                {deployLog?.groupId === group.id && deployLog.lines.length > 0 && (
                  <div className="mt-3 bg-[var(--bg)] border border-[var(--border)] rounded-lg p-3 max-h-[120px] overflow-y-auto">
                    {deployLog.lines.map((line, i) => (
                      <p key={i} className="text-[11px] font-mono text-[var(--text-secondary)]">{line}</p>
                    ))}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}

        {/* Create/Edit Group Modal */}
        {showForm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center">
            <div className="absolute inset-0 bg-black/30" onClick={() => setShowForm(false)} />
            <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-md mx-4">
              <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)]">
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                  {editing ? 'Edit Group' : 'New Group'}
                </h2>
                <button onClick={() => setShowForm(false)} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
              </div>
              <div className="px-5 py-4 space-y-4">
                <div>
                  <label className="label">Name *</label>
                  <input value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="input" placeholder="Backend Services" autoFocus />
                </div>
                <div>
                  <label className="label">Description</label>
                  <input value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} className="input" placeholder="API, database, and cache services" />
                </div>
              </div>
              <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)]">
                <button onClick={() => setShowForm(false)} className="btn btn-secondary">Cancel</button>
                <button onClick={handleSave} disabled={!form.name.trim()} className="btn btn-primary">
                  {editing ? 'Save Changes' : 'Create Group'}
                </button>
              </div>
            </div>
          </div>
        )}

        {/* Add Blueprints Modal */}
        {showAddBP && (
          <div className="fixed inset-0 z-50 flex items-center justify-center">
            <div className="absolute inset-0 bg-black/30" onClick={() => { setShowAddBP(null); setSelectedBPIds(new Set()); }} />
            <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col">
              <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border)] shrink-0">
                <h2 className="text-[15px] font-semibold text-[var(--text-primary)]">
                  Add Blueprints to Group
                </h2>
                <button onClick={() => { setShowAddBP(null); setSelectedBPIds(new Set()); }} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] text-xl">&times;</button>
              </div>
              <div className="flex-1 overflow-y-auto px-5 py-4">
                {availableForGroup(showAddBP).length === 0 ? (
                  <div className="text-center py-8">
                    <p className="text-[13px] text-[var(--text-secondary)] mb-1">No blueprints available</p>
                    <p className="text-[12px] text-[var(--text-tertiary)]">
                      All blueprints are already in groups, or none exist yet.{' '}
                      <Link href="/pipeline-blueprints" className="text-[var(--accent)] hover:underline">Create a blueprint</Link>
                    </p>
                  </div>
                ) : (
                  <div className="space-y-1.5">
                    {availableForGroup(showAddBP).map(bp => (
                      <label
                        key={bp.id}
                        className={`flex items-center gap-3 p-2.5 rounded-lg border cursor-pointer transition-all ${
                          selectedBPIds.has(bp.id)
                            ? 'border-[var(--accent)] bg-[var(--accent-subtle)]'
                            : 'border-[var(--border)] hover:border-[var(--text-tertiary)]'
                        }`}
                      >
                        <input
                          type="checkbox"
                          checked={selectedBPIds.has(bp.id)}
                          onChange={() => {
                            const next = new Set(selectedBPIds);
                            if (next.has(bp.id)) next.delete(bp.id);
                            else next.add(bp.id);
                            setSelectedBPIds(next);
                          }}
                          className="rounded"
                        />
                        <span className="text-sm">{sourceIcons[bp.source_type] || '📦'}</span>
                        <div className="flex-1 min-w-0">
                          <span className="text-[12px] font-medium text-[var(--text-primary)]">{bp.name}</span>
                          <span className="text-[10px] text-[var(--text-tertiary)] ml-2">{bp.category}</span>
                        </div>
                      </label>
                    ))}
                  </div>
                )}
              </div>
              <div className="flex justify-end gap-2 px-5 py-3 border-t border-[var(--border)] shrink-0">
                <button onClick={() => { setShowAddBP(null); setSelectedBPIds(new Set()); }} className="btn btn-secondary">Cancel</button>
                <button
                  onClick={() => handleAddBlueprints(showAddBP)}
                  disabled={selectedBPIds.size === 0}
                  className="btn btn-primary"
                >
                  Add {selectedBPIds.size > 0 ? `(${selectedBPIds.size})` : ''}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
