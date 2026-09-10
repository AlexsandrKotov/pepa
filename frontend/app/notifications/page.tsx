'use client';

import { useState, useEffect, useCallback } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import DOMPurify from 'dompurify';
import {
  notifications,
  connections,
  type NotificationRule,
  type NotificationLog,
  type NotificationStats,
  type EventCategories,
  type Connection,
  type TemplatePreset,
  type ProviderPreview,
} from '@/lib/api';
import PermissionGuard from '@/components/PermissionGuard';
import Tabs from '@/components/Tabs';

type Tab = 'overview' | 'rules' | 'history';

export default function NotificationsPage() {
  return (
    <PermissionGuard resource="notifications" action="read">
      <NotificationsContent />
    </PermissionGuard>
  );
}

function NotificationsContent() {
  const searchParams = useSearchParams();
  const router = useRouter();
  const [tab, setTab] = useState<Tab>((searchParams.get('tab') as Tab) || 'overview');
  const [toast, setToast] = useState<{ message: string; type: 'success' | 'error' } | null>(null);
  const [rulesCount, setRulesCount] = useState(0);

  useEffect(() => {
    notifications.listRules().then(r => setRulesCount(r.total ?? r.rules.length)).catch(() => {});
  }, []);

  const handleTabChange = useCallback((key: string) => {
    setTab(key as Tab);
    const params = new URLSearchParams(searchParams.toString());
    params.set('tab', key);
    router.replace(`?${params.toString()}`, { scroll: false });
  }, [searchParams, router]);

  const showToast = (message: string, type: 'success' | 'error' = 'success') => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 3000);
  };

  return (
    <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-[var(--foreground)]">Notifications</h1>
          <p className="text-sm text-[var(--muted)] mt-1">Manage notification rules, view delivery history and statistics</p>
        </div>
      </div>

      {/* Tabs */}
      <Tabs
        activeKey={tab}
        onChange={handleTabChange}
        tabs={[
          { key: 'overview', label: 'Overview', icon: 'dashboard' },
          { key: 'rules', label: 'Routing Rules', icon: 'list', badge: rulesCount || undefined },
          { key: 'history', label: 'Delivery History', icon: 'refresh' },
        ]}
        className="mb-6"
      />

      {tab === 'overview' && <OverviewTab showToast={showToast} onSwitchTab={(t) => handleTabChange(t)} />}
      {tab === 'rules' && <RulesTab showToast={showToast} />}
      {tab === 'history' && <HistoryTab />}

      {/* Toast */}
      {toast && (
        <div role="status" aria-live="polite" className={`fixed bottom-4 right-4 px-4 py-3 rounded-lg shadow-lg text-sm font-medium z-50 ${
          toast.type === 'success' ? 'bg-green-600 text-white' : 'bg-red-600 text-white'
        }`}>
          {toast.message}
        </div>
      )}
    </div>
  );
}

// ============================================================
// Overview Tab
// ============================================================

function OverviewTab({ showToast, onSwitchTab }: { showToast: (msg: string, type?: 'success' | 'error') => void; onSwitchTab: (tab: Tab) => void }) {
  const [stats, setStats] = useState<NotificationStats[]>([]);
  const [connList, setConnList] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      notifications.stats().then(data => setStats(data.stats || [])).catch(() => {}),
      connections.list({ type: 'notification' }).then(data => setConnList((data as { connections: Connection[] }).connections || [])).catch(() => {}),
    ]).finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <div className="text-[var(--muted)] text-sm py-8">Loading...</div>;
  }

  // Merge: show all notification connections, enriched with stats where available
  const connCards = connList.map(conn => {
    const provider = (conn.config?.provider as string) || '';
    const stat = stats.find(s => s.connection_id === conn.id);
    return { conn, provider, stat };
  });

  if (connCards.length === 0) {
    return (
      <div className="text-center py-12">
        <svg className="mx-auto h-12 w-12 text-[var(--muted)]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
        </svg>
        <h3 className="mt-2 text-sm font-medium text-[var(--foreground)]">No notification connections</h3>
        <p className="mt-1 text-sm text-[var(--muted)]">
          Create a notification connection on the{' '}
          <a href="/connections" className="text-[var(--accent)] hover:underline">Connections</a> page first (Slack, Telegram, Email, Teams, or Webhook).
        </p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      {connCards.map(({ conn, provider, stat }) => (
        <div key={conn.id} className="bg-[var(--surface)] border border-[var(--border)] rounded-lg p-4">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center space-x-2">
              <ProviderIcon provider={provider} />
              <div>
                <h3 className="text-sm font-medium text-[var(--foreground)]">{conn.name}</h3>
                <p className="text-xs text-[var(--muted)] capitalize">{provider}</p>
              </div>
            </div>
            {stat ? <HealthDot delivered={stat.delivered} failed={stat.failed} /> : <span className="w-2.5 h-2.5 rounded-full bg-gray-400" title="No deliveries yet" />}
          </div>
          <div className="grid grid-cols-3 gap-2 text-center">
            <div className="bg-[var(--bg)] rounded p-2">
              <p className="text-lg font-semibold text-[var(--foreground)]">{stat?.total_sent ?? 0}</p>
              <p className="text-xs text-[var(--muted)]">Total</p>
            </div>
            <div className="bg-[var(--bg)] rounded p-2">
              <p className="text-lg font-semibold text-green-500">{stat?.delivered ?? 0}</p>
              <p className="text-xs text-[var(--muted)]">Delivered</p>
            </div>
            <div className="bg-[var(--bg)] rounded p-2">
              <p className="text-lg font-semibold text-red-500">{stat?.failed ?? 0}</p>
              <p className="text-xs text-[var(--muted)]">Failed</p>
            </div>
          </div>
          {stat?.last_sent_at ? (
            <p className="text-xs text-[var(--muted)] mt-3">
              Last sent: {new Date(stat.last_sent_at).toLocaleString()}
            </p>
          ) : (
            <p className="text-xs text-[var(--muted)] mt-3 italic">No notifications sent yet</p>
          )}
          <div className="flex gap-2 mt-3 pt-3 border-t border-[var(--border)]">
            <a href="#" className="text-xs text-[var(--accent)] hover:underline" onClick={(e) => { e.preventDefault(); onSwitchTab('rules'); }}>Create rule</a>
            <span className="text-xs text-[var(--muted)]">|</span>
            <span className="text-xs text-[var(--muted)] capitalize">{conn.status}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

function ProviderIcon({ provider }: { provider: string }) {
  const colors: Record<string, string> = {
    slack: '#4A154B',
    telegram: '#0088cc',
    teams: '#6264A7',
    email: '#EA4335',
    webhook: '#6B7280',
  };
  const color = colors[provider?.toLowerCase()] || '#6B7280';
  return (
    <div className="w-8 h-8 rounded-lg flex items-center justify-center" style={{ backgroundColor: color + '20' }}>
      <span className="text-xs font-bold capitalize" style={{ color }}>{provider?.[0]?.toUpperCase() || '?'}</span>
    </div>
  );
}

function HealthDot({ delivered, failed }: { delivered: number; failed: number }) {
  const total = delivered + failed;
  if (total === 0) return <span className="w-2.5 h-2.5 rounded-full bg-gray-400" />;
  const rate = delivered / total;
  const color = rate >= 0.9 ? 'bg-green-500' : rate >= 0.7 ? 'bg-yellow-500' : 'bg-red-500';
  return <span className={`w-2.5 h-2.5 rounded-full ${color}`} />;
}

// ============================================================
// Rules Tab
// ============================================================

function RulesTab({ showToast }: { showToast: (msg: string, type?: 'success' | 'error') => void }) {
  const [rules, setRules] = useState<NotificationRule[]>([]);
  const [connList, setConnList] = useState<Connection[]>([]);
  const [eventCats, setEventCats] = useState<EventCategories | null>(null);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editingRule, setEditingRule] = useState<NotificationRule | null>(null);
  const [testing, setTesting] = useState<string | null>(null);

  const loadData = useCallback(async () => {
    try {
      const [rulesRes, connsRes, eventsRes] = await Promise.all([
        notifications.listRules(),
        connections.list({ type: 'notification' }),
        notifications.eventTypes(),
      ]);
      setRules(rulesRes.rules || []);
      setConnList((connsRes as { connections: Connection[] }).connections || []);
      setEventCats(eventsRes);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this notification rule?')) return;
    try {
      await notifications.deleteRule(id);
      setRules(prev => prev.filter(r => r.id !== id));
      showToast('Rule deleted');
    } catch (err) {
      showToast(`Delete failed: ${err}`, 'error');
    }
  };

  const handleTest = async (id: string) => {
    setTesting(id);
    try {
      const res = await notifications.testRule(id);
      showToast(`Test sent: ${res.status}`);
    } catch (err) {
      showToast(`Test failed: ${err}`, 'error');
    } finally {
      setTesting(null);
    }
  };

  const handleToggle = async (rule: NotificationRule) => {
    try {
      const updated = await notifications.updateRule(rule.id, { enabled: !rule.enabled });
      setRules(prev => prev.map(r => r.id === rule.id ? updated : r));
      showToast(updated.enabled ? 'Rule enabled' : 'Rule disabled');
    } catch (err) {
      showToast(`Toggle failed: ${err}`, 'error');
    }
  };

  if (loading) {
    return <div className="text-[var(--muted)] text-sm py-8">Loading rules...</div>;
  }

  return (
    <div>
      <div className="flex justify-end mb-4">
        <button
          onClick={() => { setEditingRule(null); setShowModal(true); }}
          className="px-4 py-2 bg-[var(--accent)] text-white text-sm font-medium rounded-lg hover:opacity-90 transition-opacity"
        >
          + Add Rule
        </button>
      </div>

      {rules.length === 0 ? (
        <div className="text-center py-12 text-[var(--muted)] text-sm">
          No routing rules configured. Click &quot;Add Rule&quot; to create one.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-[var(--border)]">
                <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Name</th>
                <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Events</th>
                <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Connection</th>
                <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Enabled</th>
                <th className="text-right py-3 px-2 text-[var(--muted)] font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rules.map(rule => {
                const conn = connList.find(c => c.id === rule.connection_id);
                const provider = conn?.config?.provider as string || '';
                return (
                  <tr key={rule.id} className="border-b border-[var(--border)] hover:bg-[var(--surface-hover)]">
                    <td className="py-3 px-2">
                      <p className="font-medium text-[var(--foreground)]">{rule.name}</p>
                      {rule.description && <p className="text-xs text-[var(--muted)]">{rule.description}</p>}
                    </td>
                    <td className="py-3 px-2">
                      <div className="flex flex-wrap gap-1">
                        {rule.event_types.slice(0, 3).map(et => (
                          <span key={et} className="px-1.5 py-0.5 text-xs rounded bg-[var(--bg)] text-[var(--muted)] border border-[var(--border)]">
                            {et}
                          </span>
                        ))}
                        {rule.event_types.length > 3 && (
                          <span className="px-1.5 py-0.5 text-xs text-[var(--muted)]">+{rule.event_types.length - 3}</span>
                        )}
                      </div>
                    </td>
                    <td className="py-3 px-2">
                      <div className="flex items-center space-x-1.5">
                        <ProviderIcon provider={provider} />
                        <span className="text-[var(--foreground)]">{conn?.name || 'Unknown'}</span>
                      </div>
                    </td>
                    <td className="py-3 px-2">
                      <button
                        onClick={() => handleToggle(rule)}
                        className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors ${
                          rule.enabled ? 'bg-[var(--accent)]' : 'bg-[var(--border)]'
                        }`}
                      >
                        <span className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                          rule.enabled ? 'translate-x-4.5' : 'translate-x-0.5'
                        }`} />
                      </button>
                    </td>
                    <td className="py-3 px-2 text-right">
                      <div className="flex justify-end space-x-1">
                        <button
                          onClick={() => handleTest(rule.id)}
                          disabled={testing === rule.id}
                          className="px-2 py-1 text-xs rounded border border-[var(--border)] text-[var(--muted)] hover:text-[var(--foreground)] hover:bg-[var(--surface-hover)] disabled:opacity-50"
                        >
                          {testing === rule.id ? '...' : 'Test'}
                        </button>
                        <button
                          onClick={() => { setEditingRule(rule); setShowModal(true); }}
                          className="px-2 py-1 text-xs rounded border border-[var(--border)] text-[var(--muted)] hover:text-[var(--foreground)] hover:bg-[var(--surface-hover)]"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() => handleDelete(rule.id)}
                          className="px-2 py-1 text-xs rounded border border-red-500/30 text-red-400 hover:bg-red-500/10"
                        >
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {showModal && (
        <RuleModal
          rule={editingRule}
          connections={connList}
          eventCategories={eventCats}
          onClose={() => setShowModal(false)}
          onSave={(rule) => {
            setRules(prev => {
              const idx = prev.findIndex(r => r.id === rule.id);
              if (idx >= 0) {
                const updated = [...prev];
                updated[idx] = rule;
                return updated;
              }
              return [...prev, rule];
            });
            setShowModal(false);
            showToast(editingRule ? 'Rule updated' : 'Rule created');
          }}
          showToast={showToast}
        />
      )}
    </div>
  );
}

// ============================================================
// Rule Modal
// ============================================================

function RuleModal({
  rule,
  connections: connList,
  eventCategories,
  onClose,
  onSave,
  showToast,
}: {
  rule: NotificationRule | null;
  connections: Connection[];
  eventCategories: EventCategories | null;
  onClose: () => void;
  onSave: (rule: NotificationRule) => void;
  showToast: (msg: string, type?: 'success' | 'error') => void;
}) {
  const isEdit = rule !== null;
  const [name, setName] = useState(rule?.name || '');
  const [description, setDescription] = useState(rule?.description || '');
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [eventTypes, setEventTypes] = useState<string[]>(rule?.event_types || []);
  const [connectionId, setConnectionId] = useState(rule?.connection_id || '');
  const [subjectTemplate, setSubjectTemplate] = useState(rule?.subject_template || '');
  const [bodyTemplate, setBodyTemplate] = useState(rule?.body_template || '');
  const [providerPreviews, setProviderPreviews] = useState<ProviderPreview[]>([]);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [saving, setSaving] = useState(false);
  const [presets, setPresets] = useState<TemplatePreset[]>([]);
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);
  const [showPresets, setShowPresets] = useState(!rule); // show presets by default when creating
  const [activePreviewTab, setActivePreviewTab] = useState<string>('');

  const selectedConn = connList.find(c => c.id === connectionId);
  const provider = (selectedConn?.config?.provider as string) || '';
  const showSubject = provider === 'email';

  // Load presets on mount
  useEffect(() => {
    notifications.presets()
      .then(data => setPresets(data.presets || []))
      .catch(() => {});
  }, []);

  // Group presets by category
  const presetsByCategory = presets.reduce<Record<string, TemplatePreset[]>>((acc, p) => {
    if (!acc[p.category]) acc[p.category] = [];
    acc[p.category].push(p);
    return acc;
  }, {});

  const handleSelectPreset = (preset: TemplatePreset) => {
    setSelectedPreset(preset.id);
    setBodyTemplate(preset.body);
    if (preset.event_types.length > 0) {
      setEventTypes(preset.event_types);
    }
    if (preset.subject && showSubject) {
      setSubjectTemplate(preset.subject);
    }
    // Auto-generate name if empty
    if (!name) {
      setName(preset.name + ' Notification');
    }
  };

  const handlePreview = async () => {
    if (!bodyTemplate) return;
    setLoadingPreview(true);
    try {
      const res = await notifications.previewAll({ body_template: bodyTemplate, event_type: eventTypes[0] || 'deployment.succeeded' });
      const previews = res.previews || [];
      setProviderPreviews(previews);
      if (!activePreviewTab && previews.length > 0) {
        setActivePreviewTab(previews[0].provider);
      }
    } catch {
      setProviderPreviews([]);
    } finally {
      setLoadingPreview(false);
    }
  };

  const handleSave = async () => {
    if (!name || !connectionId || !bodyTemplate || eventTypes.length === 0) {
      showToast('Please fill in all required fields', 'error');
      return;
    }
    setSaving(true);
    try {
      const data: Partial<NotificationRule> = {
        name,
        description,
        enabled,
        event_types: eventTypes,
        connection_id: connectionId,
        subject_template: subjectTemplate || undefined,
        body_template: bodyTemplate,
        format_config: rule?.format_config || {},
      };
      const result = isEdit
        ? await notifications.updateRule(rule.id, data)
        : await notifications.createRule(data);
      onSave(result);
    } catch (err) {
      showToast(`Save failed: ${err}`, 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={onClose}>
      <div className="bg-[var(--surface)] border border-[var(--border)] rounded-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto p-6" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-semibold text-[var(--foreground)] mb-4">{isEdit ? 'Edit Rule' : 'New Routing Rule'}</h2>

        <div className="space-y-4">
          {/* Name */}
          <div>
            <label className="block text-sm font-medium text-[var(--foreground)] mb-1">Name *</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
              placeholder="e.g., Deploy alerts to Slack"
            />
          </div>

          {/* Description */}
          <div>
            <label className="block text-sm font-medium text-[var(--foreground)] mb-1">Description</label>
            <input
              type="text"
              value={description}
              onChange={e => setDescription(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
              placeholder="Optional description"
            />
          </div>

          {/* Connection */}
          <div>
            <label className="block text-sm font-medium text-[var(--foreground)] mb-1">Notification Connection *</label>
            <select
              value={connectionId}
              onChange={e => setConnectionId(e.target.value)}
              className="w-full px-3 py-2 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
            >
              <option value="">Select a connection...</option>
              {connList.map(c => (
                <option key={c.id} value={c.id}>{c.name} ({(c.config?.provider as string) || 'unknown'})</option>
              ))}
            </select>
            {connList.length === 0 && (
              <p className="text-xs text-[var(--muted)] mt-1">
                No notification connections found.{' '}
                <a href="/connections" className="text-[var(--accent)] hover:underline">Create one</a> first.
              </p>
            )}
          </div>

          {/* Event Types */}
          <div>
            <label className="block text-sm font-medium text-[var(--foreground)] mb-1">Event Types *</label>
            {eventCategories && (
              <div className="space-y-2 max-h-48 overflow-y-auto border border-[var(--border)] rounded-lg p-3 bg-[var(--bg)]">
                {Object.entries(eventCategories.categories).map(([category, types]) => (
                  <div key={category}>
                    <p className="text-xs font-semibold text-[var(--muted)] uppercase mb-1">{category}</p>
                    <div className="flex flex-wrap gap-1">
                      {types.map(t => (
                        <button
                          key={t}
                          onClick={() => setEventTypes(prev =>
                            prev.includes(t) ? prev.filter(x => x !== t) : [...prev, t]
                          )}
                          className={`px-2 py-0.5 text-xs rounded border transition-colors ${
                            eventTypes.includes(t)
                              ? 'bg-[var(--accent)] text-white border-[var(--accent)]'
                              : 'border-[var(--border)] text-[var(--muted)] hover:text-[var(--foreground)] hover:border-[var(--accent)]'
                          }`}
                        >
                          {t.split('.').pop()}
                        </button>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {eventTypes.length > 0 && (
              <p className="text-xs text-[var(--muted)] mt-1">{eventTypes.length} event type(s) selected</p>
            )}
          </div>

          {/* Subject Template (email only) */}
          {showSubject && (
            <div>
              <label className="block text-sm font-medium text-[var(--foreground)] mb-1">Subject Template</label>
              <input
                type="text"
                value={subjectTemplate}
                onChange={e => setSubjectTemplate(e.target.value)}
                className="w-full px-3 py-2 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
                placeholder="PEPA: {{ event_type }} — {{ service_name }}"
              />
            </div>
          )}

          {/* Template Presets */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="block text-sm font-medium text-[var(--foreground)]">
                Template Presets
              </label>
              <button
                type="button"
                onClick={() => setShowPresets(!showPresets)}
                className="text-xs text-[var(--accent)] hover:underline"
              >
                {showPresets ? 'Hide presets' : 'Show presets'}
              </button>
            </div>

            {showPresets && presets.length > 0 && (
              <div className="border border-[var(--border)] rounded-lg p-3 bg-[var(--bg)] max-h-64 overflow-y-auto space-y-3">
                {Object.entries(presetsByCategory).map(([category, categoryPresets]) => (
                  <div key={category}>
                    <p className="text-xs font-semibold text-[var(--muted)] uppercase mb-1.5">{category}</p>
                    <div className="grid grid-cols-1 gap-1.5">
                      {categoryPresets.map(preset => (
                        <button
                          key={preset.id}
                          type="button"
                          onClick={() => handleSelectPreset(preset)}
                          className={`flex items-start text-left px-3 py-2 rounded-lg border transition-all ${
                            selectedPreset === preset.id
                              ? 'border-[var(--accent)] bg-[var(--accent)]/10 ring-1 ring-[var(--accent)]'
                              : 'border-[var(--border)] hover:border-[var(--accent)] hover:bg-[var(--surface)]'
                          }`}
                        >
                          <span className="text-lg mr-2 mt-0.5">{preset.icon}</span>
                          <div className="flex-1 min-w-0">
                            <p className="text-sm font-medium text-[var(--foreground)]">{preset.name}</p>
                            <p className="text-xs text-[var(--muted)] truncate">{preset.description}</p>
                          </div>
                          {selectedPreset === preset.id && (
                            <svg className="w-4 h-4 text-[var(--accent)] ml-2 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                              <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                            </svg>
                          )}
                        </button>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Body Template */}
          <div>
            <label className="block text-sm font-medium text-[var(--foreground)] mb-1">Message Template *</label>
            <textarea
              value={bodyTemplate}
              onChange={e => { setBodyTemplate(e.target.value); setSelectedPreset(null); }}
              rows={8}
              className="w-full px-3 py-2 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] font-mono focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
              placeholder={'{{ event_type }}\nService: {{ service_name }}\nStatus: {{ status }}'}
            />
            <p className="text-xs text-[var(--muted)] mt-1">
              Use {'{{ variable }}'} for dynamic values. Available: event_type, timestamp, service_name, environment, status, user, duration, url, error
            </p>
          </div>

          {/* Multi-Provider Preview */}
          <div>
            <button
              onClick={handlePreview}
              disabled={loadingPreview}
              className="px-3 py-1.5 text-xs rounded border border-[var(--border)] text-[var(--muted)] hover:text-[var(--foreground)] hover:bg-[var(--surface-hover)] disabled:opacity-50"
            >
              {loadingPreview ? 'Loading...' : 'Preview All Providers'}
            </button>

            {providerPreviews.length > 0 && (
              <div className="mt-3 border border-[var(--border)] rounded-lg overflow-hidden">
                {/* Provider tabs */}
                <div className="flex border-b border-[var(--border)] bg-[var(--bg)]">
                  {providerPreviews.map(p => (
                    <button
                      key={p.provider}
                      onClick={() => setActivePreviewTab(p.provider)}
                      className={`px-3 py-2 text-xs font-medium transition-colors ${
                        activePreviewTab === p.provider
                          ? 'text-[var(--accent)] border-b-2 border-[var(--accent)] bg-[var(--surface)]'
                          : 'text-[var(--muted)] hover:text-[var(--foreground)]'
                      }`}
                    >
                      <span className="mr-1">{p.icon}</span>
                      {p.provider}
                    </button>
                  ))}
                </div>

                {/* Active provider preview */}
                {providerPreviews.filter(p => p.provider === activePreviewTab).map(p => (
                  <div key={p.provider} className="p-4 bg-[var(--surface)]">
                    {p.format === 'html' && p.provider === 'Telegram' && (
                      <div className="bg-[#1a1a2e] rounded-lg p-4 max-w-sm">
                        <div className="flex items-start space-x-2 mb-2">
                          <div className="w-8 h-8 rounded-full bg-[#0088cc] flex items-center justify-center text-white text-xs font-bold">P</div>
                          <div className="text-sm text-white leading-relaxed" dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(p.rendered) }} />
                        </div>
                      </div>
                    )}
                    {p.format === 'html' && p.provider === 'Email' && (
                      <div className="bg-white rounded-lg border border-gray-200 overflow-hidden">
                        <div className="bg-gray-100 px-3 py-2 border-b border-gray-200">
                          <p className="text-xs text-gray-500">From: <span className="text-gray-700">notifications@pepa.platform</span></p>
                          <p className="text-xs text-gray-500">Subject: <span className="text-gray-700 font-medium">PEPA Notification</span></p>
                        </div>
                        <div className="p-4" dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(p.rendered) }} />
                      </div>
                    )}
                    {p.format === 'text' && (
                      <div className="bg-[#1a1a2e] rounded-lg p-4">
                        <div className="flex items-start space-x-2">
                          <div className="w-8 h-8 rounded bg-[#4A154B] flex items-center justify-center text-white text-xs font-bold">S</div>
                          <pre className="text-sm text-white whitespace-pre-wrap font-sans">{p.rendered}</pre>
                        </div>
                      </div>
                    )}
                    {p.format === 'json' && (
                      <pre className="text-xs text-[var(--foreground)] whitespace-pre-wrap font-mono bg-[var(--bg)] p-3 rounded border border-[var(--border)] overflow-x-auto">
                        {p.rendered}
                      </pre>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Enabled */}
          <div className="flex items-center space-x-2">
            <input
              type="checkbox"
              id="rule-enabled"
              checked={enabled}
              onChange={e => setEnabled(e.target.checked)}
              className="rounded border-[var(--border)]"
            />
            <label htmlFor="rule-enabled" className="text-sm text-[var(--foreground)]">Enabled</label>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end space-x-2 mt-6 pt-4 border-t border-[var(--border)]">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-[var(--muted)] hover:text-[var(--foreground)] rounded-lg border border-[var(--border)] hover:bg-[var(--surface-hover)]"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-2 text-sm font-medium text-white bg-[var(--accent)] rounded-lg hover:opacity-90 disabled:opacity-50"
          >
            {saving ? 'Saving...' : isEdit ? 'Update Rule' : 'Create Rule'}
          </button>
        </div>
      </div>
    </div>
  );
}

// ============================================================
// History Tab
// ============================================================

function HistoryTab() {
  const [items, setItems] = useState<NotificationLog[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState('');
  const [eventFilter, setEventFilter] = useState('');
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const perPage = 25;

  const loadHistory = useCallback(async () => {
    setLoading(true);
    const params: Record<string, string> = { page: String(page), per_page: String(perPage) };
    if (statusFilter) params.status = statusFilter;
    if (eventFilter) params.event_type = eventFilter;
    try {
      const res = await notifications.history(params);
      setItems(res.items || []);
      setTotal(res.total || 0);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [page, statusFilter, eventFilter]);

  useEffect(() => { loadHistory(); }, [loadHistory]);

  const totalPages = Math.ceil(total / perPage);

  return (
    <div>
      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-4">
        <select
          value={statusFilter}
          onChange={e => { setStatusFilter(e.target.value); setPage(1); }}
          className="px-3 py-1.5 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)]"
        >
          <option value="">All statuses</option>
          <option value="delivered">Delivered</option>
          <option value="failed">Failed</option>
          <option value="pending">Pending</option>
        </select>
        <input
          type="text"
          value={eventFilter}
          onChange={e => { setEventFilter(e.target.value); setPage(1); }}
          placeholder="Filter by event type..."
          className="px-3 py-1.5 bg-[var(--bg)] border border-[var(--border)] rounded-lg text-sm text-[var(--foreground)] focus:outline-none focus:ring-1 focus:ring-[var(--accent)] w-64"
        />
        <span className="text-sm text-[var(--muted)] self-center">{total} entries</span>
      </div>

      {loading ? (
        <div className="text-[var(--muted)] text-sm py-8">Loading history...</div>
      ) : items.length === 0 ? (
        <div className="text-center py-12 text-[var(--muted)] text-sm">No delivery logs found.</div>
      ) : (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Time</th>
                  <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Event</th>
                  <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Provider</th>
                  <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Status</th>
                  <th className="text-left py-3 px-2 text-[var(--muted)] font-medium">Preview</th>
                </tr>
              </thead>
              <tbody>
                {items.map(item => (
                  <>
                    <tr
                      key={item.id}
                      onClick={() => setExpandedId(expandedId === item.id ? null : item.id)}
                      className="border-b border-[var(--border)] hover:bg-[var(--surface-hover)] cursor-pointer"
                    >
                      <td className="py-2.5 px-2 text-[var(--muted)] whitespace-nowrap">
                        {new Date(item.sent_at).toLocaleString()}
                      </td>
                      <td className="py-2.5 px-2">
                        <span className="text-xs font-mono text-[var(--foreground)]">{item.event_type}</span>
                      </td>
                      <td className="py-2.5 px-2">
                        <span className="capitalize text-[var(--foreground)]">{item.provider}</span>
                      </td>
                      <td className="py-2.5 px-2">
                        <StatusBadge status={item.status} />
                      </td>
                      <td className="py-2.5 px-2 text-[var(--muted)] truncate max-w-xs">
                        {item.rendered_body?.split('\n')[0] || '—'}
                      </td>
                    </tr>
                    {expandedId === item.id && (
                      <tr className="border-b border-[var(--border)]">
                        <td colSpan={5} className="py-3 px-4 bg-[var(--bg)]">
                          <div className="space-y-2">
                            <div>
                              <p className="text-xs font-semibold text-[var(--muted)] mb-1">Rendered Message</p>
                              <pre className="text-xs text-[var(--foreground)] whitespace-pre-wrap font-mono bg-[var(--surface)] p-3 rounded border border-[var(--border)]">
                                {item.rendered_body || '—'}
                              </pre>
                            </div>
                            {item.rendered_subject && (
                              <div>
                                <p className="text-xs font-semibold text-[var(--muted)] mb-1">Subject</p>
                                <p className="text-xs text-[var(--foreground)]">{item.rendered_subject}</p>
                              </div>
                            )}
                            {item.response_text && (
                              <div>
                                <p className="text-xs font-semibold text-[var(--muted)] mb-1">Response</p>
                                <pre className="text-xs text-[var(--foreground)] whitespace-pre-wrap font-mono bg-[var(--surface)] p-3 rounded border border-[var(--border)]">
                                  {item.response_text}
                                </pre>
                              </div>
                            )}
                            {item.error_text && (
                              <div>
                                <p className="text-xs font-semibold text-red-400 mb-1">Error</p>
                                <pre className="text-xs text-red-300 whitespace-pre-wrap font-mono bg-[var(--surface)] p-3 rounded border border-red-500/20">
                                  {item.error_text}
                                </pre>
                              </div>
                            )}
                            <div className="flex gap-4 text-xs text-[var(--muted)]">
                              <span>ID: {item.id}</span>
                              {item.rule_id && <span>Rule: {item.rule_id}</span>}
                              <span>Connection: {item.connection_id}</span>
                              {item.delivered_at && <span>Delivered: {new Date(item.delivered_at).toLocaleString()}</span>}
                            </div>
                          </div>
                        </td>
                      </tr>
                    )}
                  </>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between mt-4 pt-4 border-t border-[var(--border)]">
              <span className="text-sm text-[var(--muted)]">Page {page} of {totalPages}</span>
              <div className="flex space-x-2">
                <button
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="px-3 py-1 text-sm rounded border border-[var(--border)] text-[var(--muted)] hover:text-[var(--foreground)] disabled:opacity-50"
                >
                  Previous
                </button>
                <button
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages}
                  className="px-3 py-1 text-sm rounded border border-[var(--border)] text-[var(--muted)] hover:text-[var(--foreground)] disabled:opacity-50"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    delivered: 'bg-green-500/10 text-green-500 border-green-500/20',
    failed: 'bg-red-500/10 text-red-500 border-red-500/20',
    pending: 'bg-yellow-500/10 text-yellow-500 border-yellow-500/20',
  };
  return (
    <span className={`px-2 py-0.5 text-xs font-medium rounded border ${styles[status] || styles.pending}`}>
      {status}
    </span>
  );
}
