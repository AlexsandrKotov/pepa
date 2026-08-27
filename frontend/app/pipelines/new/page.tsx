'use client';

import { useState, useCallback, useMemo, useEffect, memo } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import {
  ReactFlow,
  ReactFlowProvider,
  useReactFlow,
  Background,
  Controls,
  MiniMap,
  Handle,
  Position,
  MarkerType,
  addEdge,
  useNodesState,
  useEdgesState,
  type Node,
  type NodeProps,
  type Edge,
  type Connection,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { workflows, plugins, type PluginInfo, type Workflow } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import { Toast } from '@/components/Interactive';
import { autoLayout, validateDAG, generateStepName } from '@/lib/pipeline-utils';

// ── Step catalog ─────────────────────────────────────────────

interface StepData extends Record<string, unknown> {
  name: string;
  stepType: string;
  plugin: string;
  action: string;
  params: Record<string, unknown>;
}

interface StepDef {
  stepType: string;
  label: string;
  description: string;
  color: string;
  icon: string; // svg path
  plugin?: string;
  action?: string;
  defaultParams?: Record<string, unknown>;
}

const STEP_DEFS: StepDef[] = [
  {
    stepType: 'deploy',
    label: 'Deploy',
    description: 'Creates a real deployment — appears on the GitOps Workflow board',
    color: '#10B981',
    icon: 'M13 10V3L4 14h7v7l9-11h-7z',
    defaultParams: { project_name: '{{ input.project_name }}', image_tag: '{{ input.image_tag }}', stage: 'dev', team_name: '{{ input.team_name }}' },
  },
  {
    stepType: 'deploy_sim',
    label: 'Simulated Deploy',
    description: 'Dry-run deployment, no records created — safe for demos',
    color: '#6366F1',
    icon: 'M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    defaultParams: { service_name: 'my-service', namespace: 'app-dev', image: 'my-service:latest' },
  },
  {
    stepType: 'condition',
    label: 'Condition',
    description: 'Gate that continues the pipeline only when the expression is true',
    color: '#F59E0B',
    icon: 'M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01',
    defaultParams: {},
  },
  {
    stepType: 'approval',
    label: 'Approval Gate',
    description: 'Pauses execution until the release is approved',
    color: '#8B5CF6',
    icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
    defaultParams: { approvers: ['platform-team'], message: 'Approve this release' },
  },
  {
    stepType: 'notify',
    label: 'Notify (Slack)',
    description: 'Sends a message via the Slack plugin (simulated if not connected)',
    color: '#EC4899',
    icon: 'M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9',
    plugin: 'slack',
    action: 'send_message',
    defaultParams: { channel: '#deployments', text: 'Pipeline finished' },
  },
  {
    stepType: 'entity_update',
    label: 'Update Entity',
    description: 'Changes status or metadata of a catalog entity',
    color: '#14B8A6',
    icon: 'M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z',
    defaultParams: { entity_id: '', status: 'active' },
  },
  {
    stepType: 'task',
    label: 'Generic Task',
    description: 'Free-form step — pick any plugin and action',
    color: '#6B7280',
    icon: 'M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4',
    defaultParams: {},
  },
];

const stepDef = (stepType: string): StepDef =>
  STEP_DEFS.find(d => d.stepType === stepType) || STEP_DEFS[STEP_DEFS.length - 1];

// Friendly param fields shown instead of raw JSON for known step types.
const PARAM_FIELDS: Record<string, Array<{ key: string; label: string; placeholder?: string; hint?: string }>> = {
  deploy: [
    { key: 'project_name', label: 'Project Name', placeholder: 'my-service' },
    { key: 'image_tag', label: 'Image Tag', placeholder: 'latest' },
    { key: 'stage', label: 'Stage', placeholder: 'dev' },
    { key: 'team_name', label: 'Team', placeholder: 'platform-team' },
    { key: 'namespace', label: 'Namespace', placeholder: 'auto: app-<stage>' },
  ],
  deploy_sim: [
    { key: 'service_name', label: 'Service Name', placeholder: 'my-service' },
    { key: 'namespace', label: 'Namespace', placeholder: 'app-dev' },
    { key: 'image', label: 'Image', placeholder: 'my-service:latest' },
  ],
  approval: [
    { key: 'approvers', label: 'Approvers (comma-separated)', placeholder: 'platform-team, team-lead' },
    { key: 'message', label: 'Message', placeholder: 'Approve this release' },
  ],
  notify: [
    { key: 'channel', label: 'Channel', placeholder: '#deployments' },
    { key: 'text', label: 'Message', placeholder: 'Pipeline finished' },
  ],
  entity_update: [
    { key: 'entity_id', label: 'Entity ID (UUID)', placeholder: '00000000-...' },
    { key: 'status', label: 'New Status', placeholder: 'active' },
  ],
};

// ── Custom node ──────────────────────────────────────────────

const StepNode = memo(({ data, selected }: NodeProps) => {
  const d = data as unknown as StepData;
  const def = stepDef(d.stepType);
  const subtitle = d.plugin ? `${d.plugin}${d.action ? ' · ' + d.action : ''}` : def.label;
  return (
    <div
      className="rounded-lg bg-[var(--surface)] shadow-sm px-3 py-2.5 min-w-[190px] max-w-[230px] transition-shadow"
      style={{
        border: `1.5px solid ${selected ? def.color : 'var(--border)'}`,
        boxShadow: selected ? `0 0 0 3px ${def.color}22` : undefined,
      }}
    >
      <Handle type="target" position={Position.Left} className="!w-2.5 !h-2.5 !border-2 !border-[var(--surface)]" style={{ background: def.color }} />
      <div className="flex items-center gap-2">
        <div className="w-7 h-7 rounded-md flex items-center justify-center shrink-0" style={{ backgroundColor: `${def.color}1a` }}>
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke={def.color} strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d={def.icon} />
          </svg>
        </div>
        <div className="min-w-0">
          <p className="text-[12px] font-semibold text-[var(--text-primary)] truncate">{d.name || 'Unnamed step'}</p>
          <p className="text-[10px] text-[var(--text-tertiary)] truncate">{subtitle}</p>
        </div>
      </div>
      <Handle type="source" position={Position.Right} className="!w-2.5 !h-2.5 !border-2 !border-[var(--surface)]" style={{ background: def.color }} />
    </div>
  );
});
StepNode.displayName = 'StepNode';

const flowNodeTypes = { step: StepNode };

// ── Page ─────────────────────────────────────────────────────

export default function PipelineBuilderPage({ initialWorkflow }: { initialWorkflow?: Workflow }) {
  return (
    <ReactFlowProvider>
      <BuilderCanvas initialWorkflow={initialWorkflow} />
    </ReactFlowProvider>
  );
}

function BuilderCanvas({ initialWorkflow }: { initialWorkflow?: Workflow }) {
  const router = useRouter();
  const { screenToFlowPosition } = useReactFlow();
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [pluginList, setPluginList] = useState<PluginInfo[]>([]);
  const [saving, setSaving] = useState(false);
  const [showSave, setShowSave] = useState(false);
  const [wfName, setWfName] = useState(initialWorkflow?.name || '');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const editing = Boolean(initialWorkflow);

  useEffect(() => {
    plugins.list().then(data => setPluginList(data.plugins || [])).catch(() => {});
  }, []);

  // Load an existing workflow spec onto the canvas (edit mode).
  useEffect(() => {
    if (!initialWorkflow) return;
    const spec = (initialWorkflow.spec || {}) as { steps?: Array<Record<string, unknown>> };
    const steps = spec.steps || [];
    const nameToId = new Map<string, string>();
    const loaded: Node[] = steps.map((s, i) => {
      const name = (s.name as string) || `step-${i + 1}`;
      const id = `n-${i}-${name}`;
      nameToId.set(name, id);
      const rawType = (s.type as string) || 'task';
      // Normalize: plugin steps that match a known def keep their identity.
      const stepType = STEP_DEFS.some(d => d.stepType === rawType) ? rawType
        : s.plugin === 'slack' ? 'notify' : 'task';
      return {
        id,
        type: 'step',
        position: { x: 0, y: 0 },
        data: {
          name,
          stepType,
          plugin: (s.plugin as string) || (stepType === 'notify' ? 'slack' : ''),
          action: (s.action as string) || (stepType === 'notify' ? 'send_message' : ''),
          params: (s.params as Record<string, unknown>) || (stepType === 'condition' ? {} : {}),
          condition: (s.condition as string) || '',
        } as StepData,
      };
    });
    const loadedEdges: Edge[] = [];
    steps.forEach((s, i) => {
      const deps = (s.depends_on as string[]) || [];
      deps.forEach(dep => {
        const src = nameToId.get(dep);
        if (src) {
          loadedEdges.push(makeEdge(src, `n-${i}-${s.name}`));
        }
      });
    });
    // Layout by layers
    const laid = autoLayout(
      loaded.map(n => ({ id: n.id, name: (n.data as StepData).name, type: (n.data as StepData).stepType, position: n.position })),
      loadedEdges.map(e => ({ id: e.id, source: e.source, target: e.target }))
    );
    laid.forEach(l => {
      const n = loaded.find(x => x.id === l.id);
      if (n) n.position = l.position;
    });
    setNodes(loaded);
    setEdges(loadedEdges);
  }, [initialWorkflow, setNodes, setEdges]);

  const selectedNode = nodes.find(n => n.id === selectedId) || null;

  const onConnect = useCallback(
    (params: Connection) =>
      setEdges(eds => addEdge({ ...params, ...edgeStyle() }, eds)),
    [setEdges]
  );

  const addStep = useCallback((def: StepDef, position?: { x: number; y: number }) => {
    const existingNames = nodes.map(n => (n.data as StepData).name);
    const base = def.label.toLowerCase().replace(/\s+/g, '-');
    const name = generateStepName(existingNames, base);
    const pos = position ?? {
      x: 60 + (nodes.length % 3) * 260,
      y: 60 + Math.floor(nodes.length / 3) * 140,
    };
    const newNode: Node = {
      id: `${def.stepType}-${Date.now()}`,
      type: 'step',
      position: pos,
      data: {
        name,
        stepType: def.stepType,
        plugin: def.plugin || '',
        action: def.action || '',
        params: { ...(def.defaultParams || {}) },
      } as StepData,
    };
    setNodes(nds => [...nds, newNode]);
    setSelectedId(newNode.id);
  }, [nodes, setNodes]);

  // Drag from palette onto canvas
  const onPaletteDragStart = (e: React.DragEvent, def: StepDef) => {
    e.dataTransfer.setData('application/pepa-step', def.stepType);
    e.dataTransfer.effectAllowed = 'move';
  };
  const onCanvasDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const stepType = e.dataTransfer.getData('application/pepa-step');
    const def = STEP_DEFS.find(d => d.stepType === stepType);
    if (!def) return;
    addStep(def, screenToFlowPosition({ x: e.clientX, y: e.clientY }));
  };

  const updateNodeData = (nodeId: string, patch: Partial<StepData>) => {
    setNodes(nds => nds.map(n => (n.id === nodeId ? { ...n, data: { ...n.data, ...patch } } : n)));
  };

  const deleteNode = (nodeId: string) => {
    setNodes(nds => nds.filter(n => n.id !== nodeId));
    setEdges(eds => eds.filter(e => e.source !== nodeId && e.target !== nodeId));
    setSelectedId(cur => (cur === nodeId ? null : cur));
  };

  const handleAutoLayout = () => {
    const laid = autoLayout(
      nodes.map(n => ({ id: n.id, name: (n.data as StepData).name, type: (n.data as StepData).stepType, position: n.position })),
      edges.map(e => ({ id: e.id, source: e.source, target: e.target }))
    );
    setNodes(nds => nds.map(n => {
      const l = laid.find(x => x.id === n.id);
      return l ? { ...n, position: l.position } : n;
    }));
  };

  const saveWorkflow = async () => {
    const name = wfName.trim();
    if (!name) return;
    setSaving(true);
    try {
      const steps = nodes.map(n => {
        const d = n.data as StepData;
        const step: Record<string, unknown> = {
          name: d.name,
          type: d.stepType === 'notify' ? undefined : d.stepType,
          params: d.params || {},
          depends_on: edges
            .filter(e => e.target === n.id)
            .map(e => (nodes.find(x => x.id === e.source)?.data as StepData)?.name || '')
            .filter(Boolean),
        };
        if (d.plugin) step.plugin = d.plugin;
        if (d.action) step.action = d.action;
        if (d.stepType === 'condition' && (d as Record<string, unknown>).condition) {
          step.condition = (d as Record<string, unknown>).condition;
        }
        return step;
      });
      if (editing && initialWorkflow) {
        await workflows.update(initialWorkflow.id, { name, spec: { steps } });
      } else {
        await workflows.create({ name, source: 'visual', spec: { steps } });
      }
      router.push('/automation');
    } catch (err) {
      setToast({ message: friendlyError(err).message, type: 'error' });
      setShowSave(true);
    } finally {
      setSaving(false);
    }
  };

  const validationErrors = useMemo(() => {
    return validateDAG(
      nodes.map(n => ({ id: n.id, name: (n.data as StepData).name, type: (n.data as StepData).stepType, position: n.position })),
      edges.map(e => ({ id: e.id, source: e.source, target: e.target }))
    );
  }, [nodes, edges]);

  return (
    <div className="h-screen flex flex-col page-mesh-bg">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Header */}
      <div className="bg-[var(--surface)]/80 backdrop-blur-sm border-b px-6 py-4 flex items-center justify-between page-animate">
        <div>
          <h1 className="page-title-modern">{editing ? 'Edit Pipeline' : 'Pipeline Builder'}</h1>
          <p className="page-subtitle-modern">
            Click or drag steps from the left, then draw arrows between them — an arrow means “runs after”
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleAutoLayout}
            disabled={nodes.length === 0}
            className="px-4 py-2 bg-[var(--bg)] text-[var(--text-secondary)] rounded-md hover:bg-[var(--border)] text-sm font-medium disabled:opacity-50"
          >
            Auto Layout
          </button>
          <button
            onClick={() => { setWfName(initialWorkflow?.name || ''); setShowSave(true); }}
            disabled={saving || nodes.length === 0 || validationErrors.length > 0}
            className="px-4 py-2 bg-[var(--accent)] text-white rounded-md hover:opacity-90 text-sm font-medium disabled:opacity-50"
          >
            {editing ? 'Save Changes' : 'Save as Workflow'}
          </button>
          <Link href="/automation" className="px-4 py-2 bg-[var(--bg)] text-[var(--text-secondary)] rounded-md hover:bg-[var(--border)] text-sm font-medium">
            Cancel
          </Link>
        </div>
      </div>

      {validationErrors.length > 0 && (
        <div className="bg-red-500/10 border-b border-red-500/20 px-6 py-2">
          <p className="text-sm text-red-500">
            <strong>Validation errors:</strong> {validationErrors.join(', ')}
          </p>
        </div>
      )}

      <div className="flex-1 flex min-h-0">
        {/* Left Panel - Step Palette */}
        <div className="w-72 bg-[var(--surface)] border-r overflow-y-auto shrink-0">
          <div className="p-4">
            <h2 className="text-sm font-semibold text-[var(--text-primary)] mb-1">Steps</h2>
            <p className="text-[11px] text-[var(--text-tertiary)] mb-3">Click to add, or drag onto the canvas</p>
            <div className="space-y-2">
              {STEP_DEFS.map(def => (
                <button
                  key={def.stepType}
                  draggable
                  onDragStart={(e) => onPaletteDragStart(e, def)}
                  onClick={() => addStep(def)}
                  className="w-full flex items-start gap-2.5 p-3 bg-[var(--bg)] border border-[var(--border)] rounded-lg hover:border-[var(--text-tertiary)] hover:shadow-sm text-left transition-all cursor-grab active:cursor-grabbing"
                >
                  <div className="w-8 h-8 rounded-md flex items-center justify-center shrink-0 mt-0.5" style={{ backgroundColor: `${def.color}1a` }}>
                    <svg className="w-4.5 h-4.5" fill="none" viewBox="0 0 24 24" stroke={def.color} strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d={def.icon} />
                    </svg>
                  </div>
                  <div className="min-w-0">
                    <p className="text-[13px] font-medium text-[var(--text-primary)]">{def.label}</p>
                    <p className="text-[11px] text-[var(--text-tertiary)] mt-0.5 leading-snug">{def.description}</p>
                  </div>
                </button>
              ))}
            </div>

            {pluginList.length > 0 && (
              <div className="mt-6">
                <h2 className="text-sm font-semibold text-[var(--text-primary)] mb-1">Plugin Actions</h2>
                <p className="text-[11px] text-[var(--text-tertiary)] mb-3">Adds a task bound to the plugin</p>
                {pluginList.map(plugin => (
                  <button
                    key={plugin.id}
                    onClick={() => addStep({
                      stepType: 'task',
                      label: plugin.name,
                      description: 'Plugin action',
                      color: '#3B82F6',
                      icon: 'M13 10V3L4 14h7v7l9-11h-7z',
                      plugin: plugin.name,
                      action: plugin.actions?.[0] || '',
                    })}
                    className="w-full flex items-center gap-2.5 p-3 bg-blue-500/10 border border-blue-500/20 rounded-lg hover:bg-blue-500/15 text-left transition-colors mb-2"
                  >
                    <div className="w-8 h-8 rounded-md flex items-center justify-center shrink-0 bg-blue-500/10">
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="#3B82F6" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
                      </svg>
                    </div>
                    <div className="min-w-0">
                      <p className="text-[13px] font-medium text-blue-600">{plugin.name}</p>
                      <p className="text-[11px] text-blue-500/80 mt-0.5">
                        {plugin.actions?.length ? plugin.actions.join(', ') : 'Plugin action'}
                      </p>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Center - Canvas */}
        <div className="flex-1 relative min-w-0">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={flowNodeTypes}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeClick={(_, node) => setSelectedId(node.id)}
            onPaneClick={() => setSelectedId(null)}
            onDrop={onCanvasDrop}
            onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }}
            deleteKeyCode={['Backspace', 'Delete']}
            defaultEdgeOptions={edgeStyle()}
            fitView
            className="bg-[var(--bg)]"
          >
            <Background />
            <Controls />
            {nodes.length > 3 && <MiniMap pannable zoomable />}
          </ReactFlow>

          {/* Empty state overlay */}
          {nodes.length === 0 && (
            <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
              <div className="text-center max-w-md px-6">
                <div className="w-12 h-12 mx-auto rounded-xl bg-[var(--surface)] border border-[var(--border)] flex items-center justify-center mb-3 shadow-sm">
                  <svg className="w-6 h-6 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M13 10V3L4 14h7v7l9-11h-7z" />
                  </svg>
                </div>
                <p className="text-[14px] font-medium text-[var(--text-primary)]">The canvas is empty</p>
                <p className="text-[12px] text-[var(--text-tertiary)] mt-1.5 leading-relaxed">
                  1. Add steps from the left panel (click or drag).<br />
                  2. Drag from a step&apos;s right dot to another step&apos;s left dot to set the order.<br />
                  3. Click a step to configure it, then save as a workflow.
                </p>
              </div>
            </div>
          )}
        </div>

        {/* Right Panel - Step Config */}
        {selectedNode && (
          <StepConfigPanel
            node={selectedNode}
            pluginList={pluginList}
            onChange={(patch) => updateNodeData(selectedNode.id, patch)}
            onDelete={() => deleteNode(selectedNode.id)}
            onClose={() => setSelectedId(null)}
          />
        )}
      </div>

      {/* Save modal */}
      {showSave && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50" onClick={() => !saving && setShowSave(false)}>
          <div className="bg-[var(--surface)] rounded-lg shadow-xl w-full max-w-md" onClick={e => e.stopPropagation()}>
            <div className="p-4 border-b border-[var(--border)]">
              <h3 className="text-[14px] font-medium text-[var(--text-primary)]">
                {editing ? 'Save Changes' : 'Save as Workflow'}
              </h3>
              <p className="text-[12px] text-[var(--text-tertiary)] mt-0.5">
                {nodes.length} step{nodes.length === 1 ? '' : 's'} · it will appear on the Automation page, ready to run
              </p>
            </div>
            <div className="p-4 space-y-4">
              <div>
                <label className="label">Workflow Name *</label>
                <input
                  type="text"
                  value={wfName}
                  onChange={e => setWfName(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && wfName.trim() && saveWorkflow()}
                  className="input"
                  placeholder="e.g., nightly-deploy"
                  autoFocus
                />
              </div>
              <div className="flex justify-end gap-2 pt-1">
                <button onClick={() => setShowSave(false)} disabled={saving} className="btn btn-ghost">Cancel</button>
                <button onClick={saveWorkflow} disabled={saving || !wfName.trim()} className="btn btn-primary">
                  {saving ? 'Saving...' : editing ? 'Save Changes' : 'Create Workflow'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ── Step config panel ────────────────────────────────────────

function StepConfigPanel({ node, pluginList, onChange, onDelete, onClose }: {
  node: Node;
  pluginList: PluginInfo[];
  onChange: (patch: Partial<StepData>) => void;
  onDelete: () => void;
  onClose: () => void;
}) {
  const d = node.data as StepData;
  const def = stepDef(d.stepType);
  const fields = PARAM_FIELDS[d.stepType];
  const params = d.params || {};
  const [showRaw, setShowRaw] = useState(false);
  const [raw, setRaw] = useState('');
  const [rawError, setRawError] = useState('');

  const setParam = (key: string, value: unknown) => {
    const next = { ...params };
    if (key === 'approvers' && typeof value === 'string') {
      next[key] = value.split(',').map(s => s.trim()).filter(Boolean);
    } else {
      next[key] = value;
    }
    onChange({ params: next });
  };

  const paramValue = (key: string): string => {
    const v = params[key];
    if (Array.isArray(v)) return v.join(', ');
    return v == null ? '' : String(v);
  };

  return (
    <div className="w-80 bg-[var(--surface)] border-l overflow-y-auto shrink-0">
      <div className="p-4">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <div className="w-7 h-7 rounded-md flex items-center justify-center" style={{ backgroundColor: `${def.color}1a` }}>
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke={def.color} strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d={def.icon} />
              </svg>
            </div>
            <h2 className="text-sm font-semibold text-[var(--text-primary)]">{def.label}</h2>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={onDelete} className="text-red-500 hover:text-red-400 text-xs">Delete</button>
            <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        <div className="space-y-3">
          <div>
            <label className="label">Step Name *</label>
            <input
              type="text"
              value={d.name}
              onChange={e => onChange({ name: e.target.value })}
              className="input"
              placeholder="step name"
            />
            <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Must be unique — other steps reference it as a dependency.</p>
          </div>

          {/* Condition expression */}
          {d.stepType === 'condition' && (
            <div>
              <label className="label">Condition Expression</label>
              <input
                type="text"
                value={String((d as Record<string, unknown>).condition || '')}
                onChange={e => onChange({ condition: e.target.value } as unknown as Partial<StepData>)}
                className="input font-mono"
                placeholder="steps.build == completed"
              />
              <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                Form: <code>left == right</code>. Reference another step with <code>steps.&lt;name&gt; == completed</code>. Pipeline stops here if false.
              </p>
            </div>
          )}

          {/* Plugin/action for task & notify */}
          {(d.stepType === 'task' || d.stepType === 'notify') && (
            <>
              <div>
                <label className="label">Plugin</label>
                <select
                  value={d.plugin}
                  onChange={e => onChange({ plugin: e.target.value })}
                  className="input"
                >
                  <option value="">None (no-op task)</option>
                  {pluginList.map(p => (
                    <option key={p.id} value={p.name}>{p.name}</option>
                  ))}
                  {d.plugin && !pluginList.some(p => p.name === d.plugin) && (
                    <option value={d.plugin}>{d.plugin}</option>
                  )}
                </select>
              </div>
              <div>
                <label className="label">Action</label>
                <input
                  type="text"
                  value={d.action}
                  onChange={e => onChange({ action: e.target.value })}
                  className="input"
                  placeholder="e.g., send_message, sync"
                />
              </div>
            </>
          )}

          {/* Friendly params */}
          {fields && (
            <div className="space-y-3 pt-2 border-t border-[var(--border-light)]">
              <p className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wide">Parameters</p>
              {fields.map(f => (
                <div key={f.key}>
                  <label className="label">{f.label}</label>
                  <input
                    type="text"
                    value={paramValue(f.key)}
                    onChange={e => setParam(f.key, e.target.value)}
                    className="input"
                    placeholder={f.placeholder}
                  />
                  {f.hint && <p className="text-[10px] text-[var(--text-tertiary)] mt-1">{f.hint}</p>}
                </div>
              ))}
            </div>
          )}

          {/* Runtime inputs hint */}
          <div className="px-3 py-2 bg-[var(--bg)] rounded-lg border border-[var(--border-light)]">
            <p className="text-[10px] text-[var(--text-tertiary)] leading-relaxed">
              Tip: make the pipeline reusable — write <code className="font-mono text-[var(--accent)]">{'{{ input.image_tag }}'}</code> instead of a fixed value.
              When the pipeline runs (Automation → Run or GitOps → Via Pipeline), the user fills these inputs and they are substituted into the steps.
              Common names: <code className="font-mono">project_name</code>, <code className="font-mono">image_tag</code>, <code className="font-mono">stage</code>, <code className="font-mono">team_name</code>.
            </p>
          </div>

          {/* Raw JSON for advanced use */}
          <div className="pt-2 border-t border-[var(--border-light)]">
            <button onClick={() => { setRaw(JSON.stringify(params, null, 2)); setRawError(''); setShowRaw(!showRaw); }} className="text-[11px] text-[var(--accent)] hover:underline">
              {showRaw ? 'Hide raw JSON' : 'Advanced: raw JSON'}
            </button>
            {showRaw && (
              <div className="mt-2">
                <textarea
                  value={raw}
                  onChange={e => {
                    setRaw(e.target.value);
                    try {
                      const parsed = JSON.parse(e.target.value);
                      setRawError('');
                      onChange({ params: parsed });
                    } catch {
                      setRawError('Invalid JSON — changes not applied');
                    }
                  }}
                  rows={6}
                  className="input font-mono !text-[11px]"
                />
                {rawError && <p className="text-[10px] text-red-500 mt-1">{rawError}</p>}
              </div>
            )}
          </div>

          <div className="text-[11px] text-[var(--text-tertiary)] bg-[var(--bg)] rounded-lg p-2.5 leading-relaxed">
            <strong>Tip:</strong> drag from the right dot of one step to the left dot of another to make it run after. Select an arrow and press Delete to remove it.
          </div>
        </div>
      </div>
    </div>
  );
}

// ── Helpers ──────────────────────────────────────────────────

let edgeCounter = 0;
function makeEdge(source: string, target: string): Edge {
  return {
    id: `e-${source}-${target}-${edgeCounter++}`,
    source,
    target,
    ...edgeStyle(),
  };
}

function edgeStyle() {
  return {
    type: 'smoothstep',
    animated: true,
    style: { stroke: 'var(--text-tertiary)', strokeWidth: 1.5 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#9CA3AF', width: 16, height: 16 },
  };
}
