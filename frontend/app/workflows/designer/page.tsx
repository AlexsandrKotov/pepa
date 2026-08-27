'use client';

import { useState, useCallback, useMemo, useEffect, useRef, memo } from 'react';
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
import { Toast } from '@/components/Interactive';
import { autoLayout, validateDAG, generateStepName } from '@/lib/pipeline-utils';

// ── Types ────────────────────────────────────────────────────

interface WorkflowStepData extends Record<string, unknown> {
  name: string;
  stepType: string;
  plugin: string;
  action: string;
  params: Record<string, unknown>;
  condition?: string;
  timeout?: string;
  retry?: { maxRetries: number; backoff: string };
  status?: 'idle' | 'running' | 'success' | 'failed' | 'skipped';
}

interface StepDef {
  stepType: string;
  label: string;
  description: string;
  color: string;
  icon: string;
  plugin?: string;
  action?: string;
  defaultParams?: Record<string, unknown>;
}

// ── Step catalog ─────────────────────────────────────────────

const STEP_DEFS: StepDef[] = [
  {
    stepType: 'trigger',
    label: 'Trigger',
    description: 'Webhook, schedule, or manual trigger',
    color: '#3B82F6',
    icon: 'M13 10V3L4 14h7v7l9-11h-7z',
    defaultParams: { type: 'webhook', source: 'github' },
  },
  {
    stepType: 'action',
    label: 'Action',
    description: 'Run a plugin action',
    color: '#10B981',
    icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z',
    defaultParams: {},
  },
  {
    stepType: 'condition',
    label: 'Condition',
    description: 'Branch based on condition',
    color: '#F59E0B',
    icon: 'M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z',
    defaultParams: {},
  },
  {
    stepType: 'approval',
    label: 'Approval',
    description: 'Manual approval gate',
    color: '#8B5CF6',
    icon: 'M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z',
    defaultParams: { approvers: ['team-lead'], quorum: 1 },
  },
  {
    stepType: 'notification',
    label: 'Notify',
    description: 'Send notification (Slack, email)',
    color: '#EC4899',
    icon: 'M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9',
    plugin: 'slack',
    action: 'send_message',
    defaultParams: { channel: '#notifications', message: '' },
  },
  {
    stepType: 'loop',
    label: 'Loop',
    description: 'Repeat steps for each item',
    color: '#14B8A6',
    icon: 'M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15',
    defaultParams: { items: '[]', iterator: 'item' },
  },
  {
    stepType: 'subworkflow',
    label: 'Sub-workflow',
    description: 'Call another workflow',
    color: '#6366F1',
    icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10',
    defaultParams: { workflow_name: '', parameters: {} },
  },
  {
    stepType: 'end',
    label: 'End',
    description: 'Workflow completion',
    color: '#6B7280',
    icon: 'M5 3v4M3 5h4M6 17v4m-2-2h4m5-16l2.286 6.857L21 12l-5.714 2.143L13 21l-2.286-6.857L5 12l5.714-2.143L13 3z',
    defaultParams: {},
  },
];

const stepDef = (stepType: string): StepDef =>
  STEP_DEFS.find(d => d.stepType === stepType) || STEP_DEFS[1]; // default to action

// ── Param fields for known step types ────────────────────────

const PARAM_FIELDS: Record<string, Array<{ key: string; label: string; placeholder?: string; hint?: string }>> = {
  trigger: [
    { key: 'type', label: 'Trigger Type', placeholder: 'webhook | schedule | manual' },
    { key: 'source', label: 'Source', placeholder: 'github, gitlab, etc.' },
    { key: 'cron', label: 'Cron Expression', placeholder: '0 2 * * 1-5', hint: 'Only for schedule triggers' },
  ],
  action: [
    { key: 'application', label: 'Application', placeholder: 'my-app-staging' },
    { key: 'revision', label: 'Revision', placeholder: '{{ .version }}' },
    { key: 'strategy', label: 'Strategy', placeholder: 'rolling | canary | blue_green' },
  ],
  condition: [
    { key: 'expression', label: 'CEL Expression', placeholder: 'steps.build.status == "success"' },
  ],
  approval: [
    { key: 'approvers', label: 'Approvers (comma-separated)', placeholder: 'team-lead, sre-on-call' },
    { key: 'quorum', label: 'Quorum', placeholder: '2', hint: 'Number of required approvals' },
    { key: 'timeout', label: 'Timeout', placeholder: '24h' },
    { key: 'message', label: 'Message', placeholder: 'Approve deployment to production' },
  ],
  notification: [
    { key: 'channel', label: 'Channel', placeholder: '#deployments' },
    { key: 'message', label: 'Message', placeholder: 'Deployment completed' },
  ],
  loop: [
    { key: 'items', label: 'Items (JSON array or expression)', placeholder: '{{ .services }}' },
    { key: 'iterator', label: 'Iterator Variable Name', placeholder: 'item' },
  ],
  subworkflow: [
    { key: 'workflow_name', label: 'Workflow Name', placeholder: 'child-workflow-name' },
    { key: 'parameters', label: 'Parameters (JSON)', placeholder: '{"key": "value"}' },
  ],
};

// ── Custom node component ────────────────────────────────────

const WorkflowNode = memo(({ data, selected }: NodeProps) => {
  const d = data as unknown as WorkflowStepData;
  const def = stepDef(d.stepType);
  const subtitle = d.plugin ? `${d.plugin}${d.action ? ' · ' + d.action : ''}` : def.label;
  
  const statusColors = {
    idle: 'var(--text-tertiary)',
    running: '#3B82F6',
    success: '#10B981',
    failed: '#EF4444',
    skipped: '#F59E0B',
  };
  
  const statusIcons = {
    idle: '',
    running: '⟳',
    success: '✓',
    failed: '✕',
    skipped: '⊘',
  };

  return (
    <div
      className="rounded-lg bg-[var(--surface)] shadow-sm px-3 py-2.5 min-w-[200px] max-w-[240px] transition-shadow"
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
        <div className="min-w-0 flex-1">
          <p className="text-[12px] font-semibold text-[var(--text-primary)] truncate">{d.name || 'Unnamed'}</p>
          <p className="text-[10px] text-[var(--text-tertiary)] truncate">{subtitle}</p>
        </div>
        {d.status && d.status !== 'idle' && (
          <div className="w-5 h-5 rounded-full flex items-center justify-center text-[10px] font-bold" style={{ backgroundColor: statusColors[d.status] + '22', color: statusColors[d.status] }}>
            {statusIcons[d.status]}
          </div>
        )}
      </div>
      
      {/* Type badge */}
      <div className="mt-1.5 flex items-center gap-1.5">
        <span className="text-[9px] font-medium uppercase tracking-wide px-1.5 py-0.5 rounded" style={{ backgroundColor: def.color + '18', color: def.color }}>
          {def.label}
        </span>
        {d.timeout && (
          <span className="text-[9px] text-[var(--text-tertiary)]">⏱ {d.timeout}</span>
        )}
      </div>
      
      <Handle type="source" position={Position.Right} className="!w-2.5 !h-2.5 !border-2 !border-[var(--surface)]" style={{ background: def.color }} />
    </div>
  );
});
WorkflowNode.displayName = 'WorkflowNode';

const flowNodeTypes = { workflow: WorkflowNode };

// ── Page component ───────────────────────────────────────────

export default function WorkflowDesignerPage() {
  return (
    <ReactFlowProvider>
      <DesignerCanvas />
    </ReactFlowProvider>
  );
}

function DesignerCanvas() {
  const { screenToFlowPosition } = useReactFlow();
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [selectedEdgeIds, setSelectedEdgeIds] = useState<Set<string>>(new Set());
  const [pluginList, setPluginList] = useState<PluginInfo[]>([]);
  const [savedWorkflows, setSavedWorkflows] = useState<Workflow[]>([]);
  const [loadId, setLoadId] = useState('');
  const [loadingList, setLoadingList] = useState(false);
  const [view, setView] = useState<'canvas' | 'yaml'>('canvas');
  const [wfName, setWfName] = useState('Untitled Workflow');
  const [wfDescription, setWfDescription] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const initialized = useRef(false);
  const [currentWorkflowId, setCurrentWorkflowId] = useState<string | null>(null);

  // Load plugins and saved workflows
  useEffect(() => {
    plugins.list().then(data => setPluginList(data.plugins || [])).catch(() => {});
    setLoadingList(true);
    workflows.list()
      .then(data => setSavedWorkflows(data.workflows || []))
      .catch(() => {})
      .finally(() => setLoadingList(false));
  }, []);

  // Load a saved workflow
  const handleLoadWorkflow = (id: string) => {
    if (!id) return;
    const wf = savedWorkflows.find(w => w.id === id);
    if (!wf) return;
    
    setWfName(wf.name);
    setCurrentWorkflowId(wf.id);
    const spec = (wf.spec || {}) as Record<string, unknown>;
    const steps = ((spec.steps as Array<Record<string, unknown>>) || []);
    
    // Restore trigger config from spec
    const triggers = (spec.triggers as Array<Record<string, unknown>>) || [];
    const triggerCfg = triggers[0] || {};
    const triggerConfig = (triggerCfg.config as Record<string, unknown>) || {};
    
    // Restore description from spec context
    const ctx = (spec.context as Record<string, unknown>) || {};
    setWfDescription((ctx.description as string) || '');
    
    const nameToId = new Map<string, string>();
    const loaded: Node[] = [];
    
    // Add trigger node with saved config
    const triggerId = 'n-trigger-0';
    nameToId.set('trigger', triggerId);
    loaded.push({
      id: triggerId,
      type: 'workflow',
      position: { x: 0, y: 0 },
      data: {
        name: 'Trigger',
        stepType: 'trigger',
        plugin: '',
        action: '',
        params: {
          type: (triggerConfig.type as string) || (triggerCfg.type as string) || 'webhook',
          source: (triggerConfig.source as string) || '',
          ...(triggerConfig.cron ? { cron: triggerConfig.cron } : {}),
        },
      } as WorkflowStepData,
    });
    
    // Add step nodes
    steps.forEach((s, i) => {
      const name = (s.name as string) || `step-${i + 1}`;
      const nodeId = `n-${i}-${name}`;
      nameToId.set(name, nodeId);
      const rawType = (s.type as string) || 'action';
      const stepType = STEP_DEFS.some(d => d.stepType === rawType) ? rawType : 'action';
      
      // Convert retry from backend snake_case to frontend camelCase
      const retryRaw = s.retry as Record<string, unknown> | undefined;
      const retry = retryRaw
        ? { maxRetries: (retryRaw.max_retries as number) || 0, backoff: (retryRaw.backoff as string) || 'constant' }
        : undefined;
      
      loaded.push({
        id: nodeId,
        type: 'workflow',
        position: { x: 0, y: 0 },
        data: {
          name,
          stepType,
          plugin: (s.plugin as string) || '',
          action: (s.action as string) || '',
          params: (s.params as Record<string, unknown>) || {},
          condition: (s.condition as string) || '',
          timeout: (s.timeout as string) || '',
          retry,
        } as WorkflowStepData,
      });
    });
    
    // Add end node
    const endId = 'n-end-final';
    loaded.push({
      id: endId,
      type: 'workflow',
      position: { x: 0, y: 0 },
      data: {
        name: 'End',
        stepType: 'end',
        plugin: '',
        action: '',
        params: {},
      } as WorkflowStepData,
    });
    
    // Build edges
    const loadedEdges: Edge[] = [];
    steps.forEach((s, i) => {
      const deps = (s.depends_on as string[]) || [];
      const targetId = `n-${i}-${s.name}`;
      if (deps.length === 0 && i === 0) {
        loadedEdges.push(makeEdge(triggerId, targetId));
      } else if (deps.length > 0) {
        deps.forEach(dep => {
          const srcId = nameToId.get(dep);
          if (srcId) {
            loadedEdges.push(makeEdge(srcId, targetId));
          }
        });
      } else if (i > 0) {
        // Disconnected step — connect from previous step to keep graph connected
        const prevId = `n-${i - 1}-${steps[i - 1].name}`;
        loadedEdges.push(makeEdge(prevId, targetId));
      }
    });
    
    // Connect ALL leaf nodes (no outgoing edges) to End
    if (steps.length > 0) {
      const stepNodeIds = new Set(steps.map((_, i) => `n-${i}-${steps[i].name}`));
      const nodesWithOutgoing = new Set(loadedEdges.map(e => e.source));
      stepNodeIds.forEach(id => {
        if (!nodesWithOutgoing.has(id)) {
          loadedEdges.push(makeEdge(id, endId));
        }
      });
    } else {
      loadedEdges.push(makeEdge(triggerId, endId));
    }
    
    // Auto-layout
    const laid = autoLayout(
      loaded.map(n => ({ id: n.id, name: (n.data as WorkflowStepData).name, type: (n.data as WorkflowStepData).stepType, position: n.position })),
      loadedEdges.map(e => ({ id: e.id, source: e.source, target: e.target }))
    );
    laid.forEach(l => {
      const n = loaded.find(x => x.id === l.id);
      if (n) n.position = l.position;
    });
    
    setNodes(loaded);
    setEdges(loadedEdges);
    setSelectedId(null);
  };

  const handleNewWorkflow = useCallback(() => {
    setWfName('Untitled Workflow');
    setWfDescription('');
    
    const triggerNode: Node = {
      id: 'n-trigger-0',
      type: 'workflow',
      position: { x: 60, y: 150 },
      data: {
        name: 'Trigger',
        stepType: 'trigger',
        plugin: '',
        action: '',
        params: { type: 'webhook', source: 'github' },
      } as WorkflowStepData,
    };
    
    const endNode: Node = {
      id: 'n-end-final',
      type: 'workflow',
      position: { x: 360, y: 150 },
      data: {
        name: 'End',
        stepType: 'end',
        plugin: '',
        action: '',
        params: {},
      } as WorkflowStepData,
    };
    
    setNodes([triggerNode, endNode]);
    setEdges([makeEdge('n-trigger-0', 'n-end-final')]);
    setSelectedId(null);
    setLoadId('');
    setCurrentWorkflowId(null);
  }, [setNodes, setEdges]);

  // Initialize with empty workflow (only once on mount)
  useEffect(() => {
    if (!initialized.current) {
      initialized.current = true;
      handleNewWorkflow();
    }
  }, []);

  const selectedNode = nodes.find(n => n.id === selectedId) || null;

  const onConnect = useCallback(
    (params: Connection) =>
      setEdges(eds => addEdge({ ...params, ...edgeStyle() }, eds)),
    [setEdges]
  );

  const addStep = useCallback((def: StepDef, position?: { x: number; y: number }) => {
    const existingNames = nodes.map(n => (n.data as WorkflowStepData).name);
    const base = def.label.toLowerCase().replace(/\s+/g, '-');
    const name = generateStepName(existingNames, base);
    const pos = position ?? {
      x: 60 + (nodes.length % 3) * 260,
      y: 60 + Math.floor(nodes.length / 3) * 140,
    };
    const newNode: Node = {
      id: `${def.stepType}-${Date.now()}`,
      type: 'workflow',
      position: pos,
      data: {
        name,
        stepType: def.stepType,
        plugin: def.plugin || '',
        action: def.action || '',
        params: { ...(def.defaultParams || {}) },
      } as WorkflowStepData,
    };
    setNodes(nds => [...nds, newNode]);
    setSelectedId(newNode.id);
  }, [nodes, setNodes]);

  // Drag from palette
  const onPaletteDragStart = (e: React.DragEvent, def: StepDef) => {
    e.dataTransfer.setData('application/pepa-workflow-step', def.stepType);
    e.dataTransfer.effectAllowed = 'move';
  };

  const onCanvasDrop = (e: React.DragEvent) => {
    e.preventDefault();
    const stepType = e.dataTransfer.getData('application/pepa-workflow-step');
    const def = STEP_DEFS.find(d => d.stepType === stepType);
    if (!def) return;
    addStep(def, screenToFlowPosition({ x: e.clientX, y: e.clientY }));
  };

  const updateNodeData = (nodeId: string, patch: Partial<WorkflowStepData>) => {
    setNodes(nds => nds.map(n => (n.id === nodeId ? { ...n, data: { ...n.data, ...patch } } : n)));
  };

  // Helper: compute depends_on for a node, excluding trigger/end names
  const computeDependsOn = useCallback((nodeId: string): string[] => {
    return edges
      .filter(e => e.target === nodeId)
      .map(e => {
        const src = nodes.find(x => x.id === e.source);
        if (!src) return '';
        const srcType = (src.data as WorkflowStepData).stepType;
        // Exclude trigger and end nodes from depends_on
        if (srcType === 'trigger' || srcType === 'end') return '';
        return (src.data as WorkflowStepData).name;
      })
      .filter(Boolean);
  }, [nodes, edges]);

  const deleteNode = (nodeId: string) => {
    // Don't allow deleting trigger or end nodes if they're the only ones
    const node = nodes.find(n => n.id === nodeId);
    if (node && ((node.data as WorkflowStepData).stepType === 'trigger' || (node.data as WorkflowStepData).stepType === 'end')) {
      const sameTypeCount = nodes.filter(n => (n.data as WorkflowStepData).stepType === (node.data as WorkflowStepData).stepType).length;
      if (sameTypeCount <= 1) {
        setToast({ message: `Cannot delete the ${stepDef((node.data as WorkflowStepData).stepType).label} node`, type: 'error' });
        setTimeout(() => setToast(null), 3000);
        return;
      }
    }
    
    setNodes(nds => nds.filter(n => n.id !== nodeId));
    setEdges(eds => eds.filter(e => e.source !== nodeId && e.target !== nodeId));
    setSelectedId(cur => (cur === nodeId ? null : cur));
  };

  const handleAutoLayout = () => {
    const laid = autoLayout(
      nodes.map(n => ({ id: n.id, name: (n.data as WorkflowStepData).name, type: (n.data as WorkflowStepData).stepType, position: n.position })),
      edges.map(e => ({ id: e.id, source: e.source, target: e.target }))
    );
    setNodes(nds => nds.map(n => {
      const l = laid.find(x => x.id === n.id);
      return l ? { ...n, position: l.position } : n;
    }));
  };

  // Serialize a step node to backend-compatible spec (snake_case)
  const serializeStep = (n: Node): Record<string, unknown> => {
    const d = n.data as WorkflowStepData;
    const step: Record<string, unknown> = { name: d.name };
    if (d.stepType !== 'action') step.type = d.stepType;
    if (d.plugin) step.plugin = d.plugin;
    if (d.action) step.action = d.action;
    if (Object.keys(d.params || {}).length > 0) step.params = d.params;
    if (d.condition) step.condition = d.condition;
    if (d.timeout) step.timeout = d.timeout;
    if (d.retry && d.retry.maxRetries > 0) {
      step.retry = { max_retries: d.retry.maxRetries, backoff: d.retry.backoff };
    }
    const deps = computeDependsOn(n.id);
    if (deps.length > 0) step.depends_on = deps;
    return step;
  };

  // Generate YAML
  const yamlPreview = useMemo(() => {
    const steps = nodes
      .filter(n => {
        const type = (n.data as WorkflowStepData).stepType;
        return type !== 'trigger' && type !== 'end';
      })
      .map(n => serializeStep(n));

    const triggerNode = nodes.find(n => (n.data as WorkflowStepData).stepType === 'trigger');
    const triggerData = triggerNode ? (triggerNode.data as WorkflowStepData) : null;

    return `# Auto-generated from visual designer
name: ${wfName}
description: ${wfDescription}
trigger:
  type: ${triggerData?.params?.type || 'webhook'}
  ${triggerData?.params?.source ? `source: ${triggerData.params.source}` : ''}

steps:
${steps.map(s => `  - ${JSON.stringify(s, null, 4).replace(/\n/g, '\n    ')}`).join('\n\n')}
`;
  }, [nodes, edges, wfName, wfDescription]);

  // Validation
  const validationErrors = useMemo(() => {
    return validateDAG(
      nodes.map(n => ({ id: n.id, name: (n.data as WorkflowStepData).name, type: (n.data as WorkflowStepData).stepType, position: n.position })),
      edges.map(e => ({ id: e.id, source: e.source, target: e.target }))
    );
  }, [nodes, edges]);

  return (
    <div className="h-[calc(100vh-52px)] flex flex-col page-mesh-bg">
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}

      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border)] bg-[var(--surface)]/80 backdrop-blur-sm page-animate">
        <div>
          <h1 className="page-title-modern !mb-0">Workflow Designer</h1>
          <p className="page-subtitle-modern !text-[12px]">
            {wfName} — {nodes.length} nodes, {edges.length} edges
          </p>
        </div>
        <div className="flex items-center gap-2">
          <select
            value={loadId}
            onChange={e => { setLoadId(e.target.value); handleLoadWorkflow(e.target.value); }}
            className="px-2 py-1.5 text-[12px] border border-[var(--border)] rounded bg-[var(--surface)] text-[var(--text-secondary)]"
            disabled={loadingList}
          >
            <option value="">{loadingList ? 'Loading...' : 'Load saved workflow...'}</option>
            {savedWorkflows.map(wf => (
              <option key={wf.id} value={wf.id}>{wf.name}</option>
            ))}
          </select>
          <button
            onClick={handleNewWorkflow}
            className="px-3 py-1.5 text-[12px] border border-[var(--border)] rounded text-[var(--text-secondary)] hover:bg-[var(--bg)]"
          >
            New
          </button>
          <button
            onClick={handleAutoLayout}
            disabled={nodes.length === 0}
            className="px-3 py-1.5 text-[12px] border border-[var(--border)] rounded text-[var(--text-secondary)] hover:bg-[var(--bg)] disabled:opacity-50"
          >
            Auto Layout
          </button>
          <div className="flex rounded border border-[var(--border)] overflow-hidden">
            <button
              onClick={() => setView('canvas')}
              className={`px-3 py-1.5 text-[12px] ${view === 'canvas' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--surface)] text-[var(--text-secondary)]'}`}
            >
              Canvas
            </button>
            <button
              onClick={() => setView('yaml')}
              className={`px-3 py-1.5 text-[12px] ${view === 'yaml' ? 'bg-[var(--accent)] text-white' : 'bg-[var(--surface)] text-[var(--text-secondary)]'}`}
            >
              YAML
            </button>
          </div>
          {(() => {
            const serializedSteps = nodes
              .filter(n => {
                const type = (n.data as WorkflowStepData).stepType;
                return type !== 'trigger' && type !== 'end';
              })
              .map(n => serializeStep(n));
            const triggerNode = nodes.find(n => (n.data as WorkflowStepData).stepType === 'trigger');
            const td = triggerNode ? (triggerNode.data as WorkflowStepData) : null;
            return (
              <button
                disabled={!wfName.trim()}
                onClick={async () => {
                  if (!wfName.trim()) return;
                  const spec: Record<string, unknown> = {
                    steps: serializedSteps,
                    triggers: [{
                      type: (td?.params?.type as string) || 'webhook',
                      config: {
                        type: (td?.params?.type as string) || 'webhook',
                        ...(td?.params?.source ? { source: td.params.source } : {}),
                        ...(td?.params?.cron ? { cron: td.params.cron } : {}),
                      },
                    }],
                    context: { description: wfDescription },
                  };
                  try {
                    if (currentWorkflowId) {
                      await workflows.update(currentWorkflowId, { name: wfName, spec });
                    } else {
                      await workflows.create({ name: wfName, spec });
                    }
                    setToast({ message: `Workflow "${wfName}" saved`, type: 'success' });
                    setTimeout(() => setToast(null), 3000);
                  } catch (err) {
                    setToast({ message: err instanceof Error ? err.message : 'Save failed', type: 'error' });
                    setTimeout(() => setToast(null), 3000);
                  }
                }}
                className="px-3 py-1.5 text-[12px] bg-[var(--accent)] text-white rounded hover:opacity-90 disabled:opacity-50"
              >
                {currentWorkflowId ? 'Update' : 'Save'}
              </button>
            );
          })()}
        </div>
      </div>

      {validationErrors.length > 0 && (
        <div className="bg-red-500/10 border-b border-red-500/20 px-6 py-2">
          <p className="text-sm text-red-500">
            <strong>Validation errors:</strong> {validationErrors.join(', ')}
          </p>
        </div>
      )}

      <div className="flex-1 flex overflow-hidden">
        {/* Left Panel - Node Palette */}
        <div className="w-64 bg-[var(--surface)] border-r overflow-y-auto shrink-0">
          <div className="p-4">
            <h2 className="text-sm font-semibold text-[var(--text-primary)] mb-1">Node Types</h2>
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

            {/* Plugin actions */}
            {pluginList.length > 0 && (
              <div className="mt-6">
                <h2 className="text-sm font-semibold text-[var(--text-primary)] mb-1">Plugin Actions</h2>
                <p className="text-[11px] text-[var(--text-tertiary)] mb-3">Adds an action bound to the plugin</p>
                {pluginList.map(plugin => (
                  <button
                    key={plugin.id}
                    onClick={() => addStep({
                      stepType: 'action',
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

            {/* Workflow info */}
            <div className="mt-6 pt-6 border-t border-[var(--border)]">
              <h2 className="text-sm font-semibold text-[var(--text-primary)] mb-3">Workflow Info</h2>
              <div className="space-y-3">
                <div>
                  <label className="text-[10px] text-[var(--text-tertiary)] block mb-1">Name</label>
                  <input
                    value={wfName}
                    onChange={e => setWfName(e.target.value)}
                    className="w-full px-2 py-1.5 border border-[var(--border)] rounded text-[12px] bg-[var(--bg)] text-[var(--text-primary)]"
                  />
                </div>
                <div>
                  <label className="text-[10px] text-[var(--text-tertiary)] block mb-1">Description</label>
                  <textarea
                    value={wfDescription}
                    onChange={e => setWfDescription(e.target.value)}
                    className="w-full px-2 py-1.5 border border-[var(--border)] rounded text-[12px] bg-[var(--bg)] text-[var(--text-primary)] resize-none h-16"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Center - Canvas / YAML */}
        <div
          className="flex-1 relative min-w-0 outline-none"
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key !== 'Backspace' && e.key !== 'Delete') return;
            // Don't handle if focus is in an input/textarea/select
            const tag = (e.target as HTMLElement)?.tagName;
            if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;

            // Delete selected edges first
            if (selectedEdgeIds.size > 0) {
              e.preventDefault();
              setEdges(eds => eds.filter(ed => !selectedEdgeIds.has(ed.id)));
              setSelectedEdgeIds(new Set());
              return;
            }

            // Delete selected node (with protection)
            if (selectedId) {
              e.preventDefault();
              deleteNode(selectedId);
            }
          }}
        >
          {view === 'yaml' ? (
            <pre className="p-6 text-[13px] text-[var(--text-primary)] font-mono whitespace-pre-wrap leading-relaxed h-full overflow-auto bg-[var(--bg)]">
              {yamlPreview}
            </pre>
          ) : (
            <>
              <ReactFlow
                nodes={nodes}
                edges={edges}
                nodeTypes={flowNodeTypes}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onConnect={onConnect}
                onNodeClick={(_, node) => setSelectedId(node.id)}
                onPaneClick={() => { setSelectedId(null); setSelectedEdgeIds(new Set()); }}
                onSelectionChange={(selection) => {
                  setSelectedEdgeIds(new Set(selection.edges?.map(e => e.id) || []));
                  if (selection.nodes?.length === 1) {
                    setSelectedId(selection.nodes[0].id);
                  } else if (selection.nodes?.length === 0 && selection.edges?.length === 0) {
                    setSelectedId(null);
                  }
                }}
                onDrop={onCanvasDrop}
                onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move'; }}
                deleteKeyCode={null}
                defaultEdgeOptions={edgeStyle()}
                fitView
                className="bg-[var(--bg)]"
              >
                <Background />
                <Controls />
                {nodes.length > 3 && <MiniMap pannable zoomable nodeColor={(n) => stepDef((n.data as WorkflowStepData).stepType).color} />}
              </ReactFlow>

              {/* Empty state */}
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
                      1. Add nodes from the left panel (click or drag).<br />
                      2. Drag from a node&apos;s right dot to another node&apos;s left dot to connect them.<br />
                      3. Click a node to configure it, then save as a workflow.
                    </p>
                  </div>
                </div>
              )}
            </>
          )}
        </div>

        {/* Right Panel - Properties */}
        {selectedNode && (
          <NodeConfigPanel
            node={selectedNode}
            pluginList={pluginList}
            onChange={(patch) => updateNodeData(selectedNode.id, patch)}
            onDelete={() => deleteNode(selectedNode.id)}
            onClose={() => setSelectedId(null)}
          />
        )}
      </div>
    </div>
  );
}

// ── Node configuration panel ─────────────────────────────────

function NodeConfigPanel({ node, pluginList, onChange, onDelete, onClose }: {
  node: Node;
  pluginList: PluginInfo[];
  onChange: (patch: Partial<WorkflowStepData>) => void;
  onDelete: () => void;
  onClose: () => void;
}) {
  const d = node.data as WorkflowStepData;
  const def = stepDef(d.stepType);
  const fields = PARAM_FIELDS[d.stepType];
  const params = d.params || {};
  const [showRaw, setShowRaw] = useState(false);
  const [raw, setRaw] = useState('');
  const [rawError, setRawError] = useState('');

  // Reset raw JSON state when switching nodes
  useEffect(() => {
    setShowRaw(false);
    setRaw('');
    setRawError('');
  }, [node.id]);

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
            <label className="label">Node Name *</label>
            <input
              type="text"
              value={d.name}
              onChange={e => onChange({ name: e.target.value })}
              className="input"
              placeholder="node name"
            />
            <p className="text-[10px] text-[var(--text-tertiary)] mt-1">Must be unique — other nodes reference it as a dependency.</p>
          </div>

          {/* Condition expression */}
          {d.stepType === 'condition' && (
            <div>
              <label className="label">Condition Expression (CEL)</label>
              <input
                type="text"
                value={d.condition || ''}
                onChange={e => onChange({ condition: e.target.value })}
                className="input font-mono"
                placeholder="steps.build.status == &quot;success&quot;"
              />
              <p className="text-[10px] text-[var(--text-tertiary)] mt-1">
                Use CEL expressions. Reference other steps with <code>steps.&lt;name&gt;.status</code>.
              </p>
            </div>
          )}

          {/* Plugin/action for action & notification */}
          {(d.stepType === 'action' || d.stepType === 'notification') && (
            <>
              <div>
                <label className="label">Plugin</label>
                <select
                  value={d.plugin}
                  onChange={e => onChange({ plugin: e.target.value })}
                  className="input"
                >
                  <option value="">None</option>
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
                  placeholder="e.g., deploy, send_message"
                />
              </div>
            </>
          )}

          {/* Timeout */}
          <div>
            <label className="label">Timeout</label>
            <input
              type="text"
              value={d.timeout || ''}
              onChange={e => onChange({ timeout: e.target.value })}
              className="input"
              placeholder="e.g., 5m, 1h"
            />
          </div>

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

          {/* Retry config */}
          <div className="pt-2 border-t border-[var(--border-light)]">
            <p className="text-[11px] font-medium text-[var(--text-tertiary)] uppercase tracking-wide mb-2">Retry Policy</p>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)]">Max Retries</label>
                <input
                  type="number"
                  value={d.retry?.maxRetries ?? 0}
                  onChange={e => onChange({ retry: { maxRetries: parseInt(e.target.value) || 0, backoff: d.retry?.backoff || 'constant' } })}
                  className="input !text-[11px]"
                  min="0"
                  max="10"
                />
              </div>
              <div>
                <label className="text-[10px] text-[var(--text-tertiary)]">Backoff</label>
                <select
                  value={d.retry?.backoff || 'constant'}
                  onChange={e => onChange({ retry: { maxRetries: d.retry?.maxRetries ?? 0, backoff: e.target.value } })}
                  className="input !text-[11px]"
                >
                  <option value="constant">Constant</option>
                  <option value="exponential">Exponential</option>
                  <option value="linear">Linear</option>
                </select>
              </div>
            </div>
          </div>

          {/* Raw JSON */}
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

          {/* Tips */}
          <div className="text-[11px] text-[var(--text-tertiary)] bg-[var(--bg)] rounded-lg p-2.5 leading-relaxed">
            <strong>Tip:</strong> drag from the right dot of one node to the left dot of another to create a dependency. Click an edge to select it, then press Delete to remove it.
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
