'use client';

import { useState, useEffect, useRef, type ReactNode } from 'react';
import { entities, workflows, plugins, scorecards, ai, type Entity, type AIChatResponse } from '@/lib/api';

/* ─── Modal ──────────────────────────────────────────────── */

export function Modal({ open, onClose, title, children }: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
}) {
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/20" onClick={onClose} />
      <div className="relative bg-[var(--surface)] border border-[var(--border)] rounded-lg shadow-lg w-full max-w-md mx-4">
        <div className="flex items-center justify-between px-4 py-3 border-b border-[var(--border-light)]">
          <h2 className="text-[14px] font-medium text-[var(--text-primary)]">{title}</h2>
          <button onClick={onClose} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)] transition-colors">
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="px-4 py-4">{children}</div>
      </div>
    </div>
  );
}

/* ─── Toast ──────────────────────────────────────────────── */

export function Toast({ message, type, onClose }: { message: string; type: 'success' | 'error'; onClose: () => void }) {
  useEffect(() => {
    const t = setTimeout(onClose, 3000);
    return () => clearTimeout(t);
  }, [onClose]);

  return (
    <div className={`fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-[100] px-6 py-3 rounded-lg shadow-2xl text-[14px] font-medium animate-in fade-in duration-200 ${
      type === 'success' ? 'bg-[var(--accent)] text-white' : 'bg-red-600 text-white'
    }`}>
      {message}
    </div>
  );
}

/* ─── Entity Create Form ─────────────────────────────────── */

export function EntityCreateButton({ entityTypes, onCreated }: {
  entityTypes: { type_key: string; display_name: string }[];
  onCreated?: (entity: Entity) => void;
}) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [form, setForm] = useState({ name: '', type_key: '', description: '', metadata: '' });

  const submit = async () => {
    if (!form.name || !form.type_key) return;
    setLoading(true);
    try {
      const payload: Record<string, unknown> = {
        name: form.name,
        type_key: form.type_key,
        description: form.description || undefined,
      };
      if (form.metadata) {
        try { payload.metadata = JSON.parse(form.metadata); } catch { /* ignore */ }
      }
      const entity = await entities.create(payload);
      setToast({ message: `Entity "${entity.name}" created`, type: 'success' });
      setForm({ name: '', type_key: '', description: '', metadata: '' });
      setOpen(false);
      onCreated?.(entity);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to create', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button onClick={() => setOpen(true)} className="btn btn-primary btn-sm">
        New Entity
      </button>

      <Modal open={open} onClose={() => setOpen(false)} title="Create Entity">
        <div className="space-y-3">
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Name *</label>
            <input className="input" placeholder="entity-name" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
          </div>
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Type *</label>
            <select className="select" value={form.type_key} onChange={e => setForm({ ...form, type_key: e.target.value })}>
              <option value="">Select...</option>
              {entityTypes.map(t => <option key={t.type_key} value={t.type_key}>{t.display_name}</option>)}
            </select>
          </div>
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Description</label>
            <input className="input" placeholder="Optional description" value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} />
          </div>
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Metadata (JSON)</label>
            <textarea className="input min-h-[60px] text-mono" placeholder='{"key": "value"}' value={form.metadata} onChange={e => setForm({ ...form, metadata: e.target.value })} />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={() => setOpen(false)} className="btn btn-secondary btn-sm">Cancel</button>
            <button onClick={submit} disabled={loading || !form.name || !form.type_key} className="btn btn-primary btn-sm">
              {loading ? 'Creating...' : 'Create'}
            </button>
          </div>
        </div>
      </Modal>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── Entity Edit Form ───────────────────────────────────── */

export function EntityEditButton({ entity, onUpdated }: {
  entity: Entity;
  onUpdated?: (entity: Entity) => void;
}) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [form, setForm] = useState({
    name: entity.name,
    description: entity.description || '',
    status: entity.status,
    metadata: entity.metadata ? JSON.stringify(entity.metadata, null, 2) : '',
  });

  const submit = async () => {
    setLoading(true);
    try {
      const payload: Record<string, unknown> = { name: form.name, description: form.description, status: form.status };
      if (form.metadata) {
        try { payload.metadata = JSON.parse(form.metadata); } catch { /* ignore */ }
      }
      const updated = await entities.update(entity.id, payload);
      setToast({ message: `Entity updated`, type: 'success' });
      setOpen(false);
      onUpdated?.(updated);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Failed to update', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button onClick={() => setOpen(true)} className="btn btn-secondary btn-sm">Edit</button>

      <Modal open={open} onClose={() => setOpen(false)} title="Edit Entity">
        <div className="space-y-3">
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Name</label>
            <input className="input" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
          </div>
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Description</label>
            <input className="input" value={form.description} onChange={e => setForm({ ...form, description: e.target.value })} />
          </div>
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Status</label>
            <select className="select" value={form.status} onChange={e => setForm({ ...form, status: e.target.value })}>
              <option value="active">Active</option>
              <option value="deprecated">Deprecated</option>
              <option value="development">Development</option>
            </select>
          </div>
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Metadata (JSON)</label>
            <textarea className="input min-h-[60px] text-mono" value={form.metadata} onChange={e => setForm({ ...form, metadata: e.target.value })} />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={() => setOpen(false)} className="btn btn-secondary btn-sm">Cancel</button>
            <button onClick={submit} disabled={loading} className="btn btn-primary btn-sm">
              {loading ? 'Saving...' : 'Save'}
            </button>
          </div>
        </div>
      </Modal>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── Workflow Execute Button ────────────────────────────── */

export function WorkflowExecuteButton({ workflowId, workflowName, onExecuted }: {
  workflowId: string;
  workflowName: string;
  onExecuted?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [params, setParams] = useState('');

  const execute = async () => {
    setLoading(true);
    try {
      let parsedParams: Record<string, unknown> | undefined;
      if (params.trim()) {
        try { parsedParams = JSON.parse(params); } catch { /* ignore */ }
      }
      await workflows.execute(workflowId, parsedParams);
      setToast({ message: `Workflow executed`, type: 'success' });
      setOpen(false);
      setParams('');
      onExecuted?.();
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Execution failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button onClick={() => setOpen(true)} className="btn btn-primary btn-sm">Execute</button>

      <Modal open={open} onClose={() => setOpen(false)} title={`Execute: ${workflowName}`}>
        <div className="space-y-3">
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Parameters (JSON, optional)</label>
            <textarea
              className="input min-h-[60px] text-mono"
              placeholder='{"version": "v1.0"}'
              value={params}
              onChange={e => setParams(e.target.value)}
            />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={() => setOpen(false)} className="btn btn-secondary btn-sm">Cancel</button>
            <button onClick={execute} disabled={loading} className="btn btn-primary btn-sm">
              {loading ? 'Running...' : 'Run'}
            </button>
          </div>
        </div>
      </Modal>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── Plugin Toggle ──────────────────────────────────────── */

export function PluginToggle({ pluginName, enabled, onToggle }: {
  pluginName: string;
  enabled: boolean;
  onToggle?: (enabled: boolean) => void;
}) {
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const toggle = async () => {
    setLoading(true);
    try {
      if (enabled) {
        await plugins.disable(pluginName);
      } else {
        await plugins.enable(pluginName);
      }
      onToggle?.(!enabled);
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Toggle failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button
        onClick={toggle}
        disabled={loading}
        className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
          enabled ? 'bg-[var(--accent)]' : 'bg-[var(--border)]'
        } ${loading ? 'opacity-50' : ''}`}
      >
        <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
          enabled ? 'translate-x-[18px]' : 'translate-x-[3px]'
        }`} />
      </button>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── Scorecard Evaluate Button ──────────────────────────── */

export function ScorecardEvaluateButton({ scorecardId, entities: ents, onEvaluated }: {
  scorecardId: string;
  entities: { id: string; name: string }[];
  onEvaluated?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [selectedEntity, setSelectedEntity] = useState('');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const evaluate = async () => {
    if (!selectedEntity) return;
    setLoading(true);
    try {
      const result = await scorecards.evaluate(scorecardId, selectedEntity);
      const pct = result.max_score > 0 ? Math.round((result.score / result.max_score) * 100) : 0;
      setToast({ message: `Score: ${result.score}/${result.max_score} (${pct}%) — ${result.level}`, type: 'success' });
      setOpen(false);
      onEvaluated?.();
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Evaluation failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button onClick={() => setOpen(true)} className="btn btn-primary btn-sm">Evaluate</button>

      <Modal open={open} onClose={() => setOpen(false)} title="Evaluate Scorecard">
        <div className="space-y-3">
          <div>
            <label className="block text-[12px] text-[var(--text-secondary)] mb-1">Select Entity</label>
            <select className="select" value={selectedEntity} onChange={e => setSelectedEntity(e.target.value)}>
              <option value="">Choose...</option>
              {ents.map(e => <option key={e.id} value={e.id}>{e.name}</option>)}
            </select>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button onClick={() => setOpen(false)} className="btn btn-secondary btn-sm">Cancel</button>
            <button onClick={evaluate} disabled={loading || !selectedEntity} className="btn btn-primary btn-sm">
              {loading ? 'Evaluating...' : 'Evaluate'}
            </button>
          </div>
        </div>
      </Modal>

      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── Marketplace Install Button ─────────────────────────── */

export function MarketplaceInstallButton({ pluginName, version, type, onInstalled }: {
  pluginName: string;
  version: string;
  type: string;
  onInstalled?: () => void;
}) {
  const [loading, setLoading] = useState(false);
  const [installed, setInstalled] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const install = async () => {
    setLoading(true);
    try {
      await plugins.install({ name: pluginName, version, type, enabled: true });
      setInstalled(true);
      setToast({ message: `${pluginName} installed successfully`, type: 'success' });
      onInstalled?.();
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Install failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button
        onClick={install}
        disabled={loading || installed}
        className={`px-2.5 py-1 text-[11px] rounded ${
          installed
            ? 'bg-emerald-500/10 text-emerald-600 border border-emerald-500/20 cursor-default'
            : loading
              ? 'bg-[var(--text-tertiary)] text-white cursor-wait'
              : 'bg-[var(--accent)] text-white hover:opacity-90'
        }`}
      >
        {installed ? 'Installed' : loading ? 'Installing...' : 'Install'}
      </button>
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── Workflow Save Button ───────────────────────────────── */

export function WorkflowSaveButton({ workflow, onSaved }: {
  workflow: { name: string; description: string; spec: Record<string, unknown> };
  onSaved?: () => void;
}) {
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const save = async () => {
    setLoading(true);
    try {
      await workflows.create(workflow as unknown as Record<string, unknown>);
      setToast({ message: `Workflow "${workflow.name}" saved`, type: 'success' });
      onSaved?.();
    } catch (err) {
      setToast({ message: err instanceof Error ? err.message : 'Save failed', type: 'error' });
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <button
        onClick={save}
        disabled={loading}
        className="px-3 py-1.5 text-[12px] bg-[var(--accent)] text-white rounded hover:opacity-90 disabled:opacity-50"
      >
        {loading ? 'Saving...' : 'Save Workflow'}
      </button>
      {toast && <Toast message={toast.message} type={toast.type} onClose={() => setToast(null)} />}
    </>
  );
}

/* ─── AI Chat Component ──────────────────────────────────── */

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  model?: string;
  tokens?: number;
}

export function AIChatPanel() {
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  const send = async () => {
    if (!input.trim() || loading) return;
    const userMsg: ChatMessage = { role: 'user', content: input.trim() };
    setMessages(prev => [...prev, userMsg]);
    setInput('');
    setLoading(true);
    setError(null);

    try {
      const res = await ai.chat({ message: userMsg.content, enable_tools: true });
      const botMsg: ChatMessage = {
        role: 'assistant',
        content: res.response,
        model: res.model,
        tokens: res.tokens_used?.total_tokens,
      };
      setMessages(prev => [...prev, botMsg]);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Chat failed';
      setError(msg);
      setMessages(prev => [...prev, {
        role: 'assistant',
        content: `No LLM provider is configured. Add an AI provider to enable chat.\n\nError: ${msg}`,
      }]);
    } finally {
      setLoading(false);
    }
  };

  const suggestions = [
    'What services are deployed?',
    'Show me recent workflow executions',
    'Which plugins are enabled?',
    'What is the status of payment-api?',
  ];

  return (
    <div className="flex flex-col h-[500px] border border-[var(--border)] rounded-lg bg-[var(--surface)]">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <div className="w-12 h-12 rounded-full bg-[var(--border-light)] flex items-center justify-center mb-3">
              <svg className="w-6 h-6 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
              </svg>
            </div>
            <p className="text-[13px] text-[var(--text-secondary)] mb-4">Ask PEPA about your platform</p>
            <div className="flex flex-wrap gap-2 justify-center max-w-sm">
              {suggestions.map(s => (
                <button
                  key={s}
                  onClick={() => { setInput(s); }}
                  className="px-3 py-1.5 text-[12px] bg-[var(--border-light)] text-[var(--text-secondary)] rounded-full hover:bg-[var(--border)] transition-colors"
                >
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}

        {messages.map((msg, i) => (
          <div key={i} className={`flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            <div className={`max-w-[80%] px-4 py-2.5 rounded-2xl text-[13px] leading-relaxed ${
              msg.role === 'user'
                ? 'bg-[var(--accent)] text-white rounded-br-md'
                : 'bg-[var(--bg)] text-[var(--text-primary)] rounded-bl-md border border-[var(--border-light)]'
            }`}>
              <p className="whitespace-pre-wrap">{msg.content}</p>
              {msg.model && (
                <div className="flex items-center gap-2 mt-1.5 text-[10px] opacity-50">
                  <span>{msg.model}</span>
                  {msg.tokens && <span>{msg.tokens} tokens</span>}
                </div>
              )}
            </div>
          </div>
        ))}

        {loading && (
          <div className="flex justify-start">
            <div className="bg-[var(--bg)] rounded-2xl rounded-bl-md px-4 py-3 border border-[var(--border-light)]">
              <div className="flex gap-1">
                <span className="w-2 h-2 bg-[var(--text-tertiary)] rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
                <span className="w-2 h-2 bg-[var(--text-tertiary)] rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
                <span className="w-2 h-2 bg-[var(--text-tertiary)] rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-[var(--border-light)] p-3">
        <div className="flex items-center gap-2">
          <input
            value={input}
            onChange={e => setInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && !e.shiftKey && send()}
            placeholder="Ask about your platform..."
            className="flex-1 px-3 py-2 border border-[var(--border)] rounded-lg text-[13px] text-[var(--text-primary)] focus:outline-none focus:border-[var(--accent)] transition-colors"
            disabled={loading}
          />
          <button
            onClick={send}
            disabled={loading || !input.trim()}
            className="px-4 py-2 bg-[var(--accent)] text-white rounded-lg text-[13px] hover:opacity-90 disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
}
