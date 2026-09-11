'use client';

import { useState, useEffect } from 'react';
import { autoDeployRules, environments, type AutoDeployRule, type Environment } from '@/lib/api';

export default function AutoDeployPage() {
  const [rules, setRules] = useState<AutoDeployRule[]>([]);
  const [envs, setEnvs] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [editingRule, setEditingRule] = useState<AutoDeployRule | null>(null);

  // Form state
  const [formBranch, setFormBranch] = useState('');
  const [formEnv, setFormEnv] = useState('');
  const [formProjectPath, setFormProjectPath] = useState('');
  const [formImageTagSource, setFormImageTagSource] = useState('branch_name');
  const [formImageTagRegex, setFormImageTagRegex] = useState('');
  const [formImageName, setFormImageName] = useState('');
  const [formRequirePipeline, setFormRequirePipeline] = useState(false);

  const loadRules = async () => {
    try {
      const res = await autoDeployRules.list();
      setRules(res.rules || []);
    } catch {
      setError('Failed to load auto-deploy rules');
    } finally {
      setLoading(false);
    }
  };

  const loadEnvs = async () => {
    try {
      const res = await environments.list();
      setEnvs(res.environments || []);
    } catch { /* ignore */ }
  };

  useEffect(() => {
    loadRules();
    loadEnvs();
  }, []);

  const resetForm = () => {
    setFormBranch('');
    setFormEnv('');
    setFormProjectPath('');
    setFormImageTagSource('branch_name');
    setFormImageTagRegex('');
    setFormImageName('');
    setFormRequirePipeline(false);
    setEditingRule(null);
    setShowForm(false);
  };

  const handleSave = async () => {
    if (!formBranch || !formEnv) return;
    try {
      const data: Partial<AutoDeployRule> = {
        branch_pattern: formBranch,
        environment_id: formEnv,
        project_path: formProjectPath || undefined,
        image_tag_source: formImageTagSource,
        image_tag_regex: formImageTagRegex || undefined,
        image_name: formImageName || undefined,
        require_pipeline_success: formRequirePipeline,
        enabled: true,
      };

      if (editingRule) {
        await autoDeployRules.update(editingRule.id, data);
      } else {
        await autoDeployRules.create(data);
      }
      resetForm();
      await loadRules();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save rule');
    }
  };

  const handleEdit = (rule: AutoDeployRule) => {
    setEditingRule(rule);
    setFormBranch(rule.branch_pattern);
    setFormEnv(rule.environment_id);
    setFormProjectPath(rule.project_path || '');
    setFormImageTagSource(rule.image_tag_source);
    setFormImageTagRegex(rule.image_tag_regex || '');
    setFormImageName(rule.image_name || '');
    setFormRequirePipeline(rule.require_pipeline_success);
    setShowForm(true);
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this auto-deploy rule?')) return;
    try {
      await autoDeployRules.delete(id);
      setRules(rules.filter(r => r.id !== id));
    } catch {
      setError('Failed to delete rule');
    }
  };

  const handleToggle = async (rule: AutoDeployRule) => {
    try {
      await autoDeployRules.update(rule.id, { enabled: !rule.enabled });
      await loadRules();
    } catch {
      setError('Failed to toggle rule');
    }
  };

  const getEnvBadge = (rule: AutoDeployRule) => {
    if (!rule.env_name) return null;
    return (
      <span
        className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium"
        style={{
          backgroundColor: (rule.env_color || '#6B7280') + '20',
          color: rule.env_color || '#6B7280',
        }}
      >
        <span className="w-2 h-2 rounded-full" style={{ backgroundColor: rule.env_color || '#6B7280' }} />
        {rule.env_name}
      </span>
    );
  };

  if (loading) {
    return (
      <div className="space-y-4">
        <div className="h-8 bg-gray-200 dark:bg-gray-700 rounded animate-pulse w-48" />
        <div className="h-32 bg-gray-200 dark:bg-gray-700 rounded animate-pulse" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-bold text-gray-900 dark:text-white">Auto-Deploy Rules</h2>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            Configure branch-to-environment mappings for webhook-triggered auto-deploys.
            When a GitLab push matches a rule, PEPA writes the image tag to the manifest repo.
          </p>
        </div>
        <button
          onClick={() => { resetForm(); setShowForm(true); }}
          className="px-4 py-2 bg-emerald-600 text-white rounded-lg hover:bg-emerald-700 text-sm font-medium"
        >
          + Add Rule
        </button>
      </div>

      {error && (
        <div className="p-3 rounded-lg bg-red-50 dark:bg-red-900/20 text-red-700 dark:text-red-300 text-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={() => setError('')} className="text-red-500 hover:text-red-700">&times;</button>
        </div>
      )}

      {/* Webhook Info */}
      <div className="p-4 rounded-xl bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800">
        <h3 className="text-sm font-semibold text-blue-800 dark:text-blue-300 mb-1">Webhook URL</h3>
        <code className="text-xs text-blue-700 dark:text-blue-400 bg-blue-100 dark:bg-blue-900/40 px-2 py-1 rounded">
          POST /api/v1/webhooks/gitlab
        </code>
        <p className="text-xs text-blue-600 dark:text-blue-400 mt-2">
          Configure this URL in your GitLab project settings under Webhooks. Set the trigger to &ldquo;Push events&rdquo;.
          Optionally set a secret token for authentication.
        </p>
      </div>

      {/* Create/Edit Form */}
      {showForm && (
        <div className="p-5 rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 space-y-4">
          <h3 className="text-base font-semibold text-gray-900 dark:text-white">
            {editingRule ? 'Edit Rule' : 'New Auto-Deploy Rule'}
          </h3>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Branch Pattern *</label>
              <input
                type="text"
                value={formBranch}
                onChange={e => setFormBranch(e.target.value)}
                placeholder="e.g. main, release/*, testing/*"
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
              />
              <p className="text-xs text-gray-500 mt-1">Glob pattern: * matches any chars, ? matches single char</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Target Environment *</label>
              <select
                value={formEnv}
                onChange={e => setFormEnv(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
              >
                <option value="">Select environment...</option>
                {envs.map(env => (
                  <option key={env.id} value={env.id}>{env.name} ({env.slug})</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Project Path (optional)</label>
              <input
                type="text"
                value={formProjectPath}
                onChange={e => setFormProjectPath(e.target.value)}
                placeholder="e.g. group/repo"
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
              />
              <p className="text-xs text-gray-500 mt-1">Leave empty to match all projects</p>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Image Tag Source</label>
              <select
                value={formImageTagSource}
                onChange={e => setFormImageTagSource(e.target.value)}
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
              >
                <option value="branch_name">Branch name (sanitized)</option>
                <option value="regex">Regex extraction from branch</option>
              </select>
            </div>
            {formImageTagSource === 'regex' && (
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Tag Regex</label>
                <input
                  type="text"
                  value={formImageTagRegex}
                  onChange={e => setFormImageTagRegex(e.target.value)}
                  placeholder="e.g. release/(.*)"
                  className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
                />
                <p className="text-xs text-gray-500 mt-1">First capture group becomes the image tag</p>
              </div>
            )}
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Image Name (optional)</label>
              <input
                type="text"
                value={formImageName}
                onChange={e => setFormImageName(e.target.value)}
                placeholder="e.g. registry.example.com/myapp"
                className="w-full px-3 py-2 rounded-lg border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-white text-sm"
              />
            </div>
          </div>

          <label className="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              type="checkbox"
              checked={formRequirePipeline}
              onChange={e => setFormRequirePipeline(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600"
            />
            Require CI pipeline success before deploying
          </label>

          <div className="flex gap-2">
            <button
              onClick={handleSave}
              disabled={!formBranch || !formEnv}
              className="px-4 py-2 bg-emerald-600 text-white rounded-lg hover:bg-emerald-700 disabled:opacity-50 text-sm font-medium"
            >
              {editingRule ? 'Update' : 'Create'}
            </button>
            <button
              onClick={resetForm}
              className="px-4 py-2 bg-gray-100 text-gray-700 rounded-lg hover:bg-gray-200 dark:bg-gray-700 dark:text-gray-200 text-sm"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Rules Table */}
      {rules.length === 0 ? (
        <div className="text-center py-12 bg-gray-50 dark:bg-gray-800 rounded-xl">
          <svg className="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
          </svg>
          <h3 className="mt-2 text-lg font-medium text-gray-900 dark:text-white">No auto-deploy rules</h3>
          <p className="mt-1 text-gray-500 dark:text-gray-400 text-sm">
            Create a rule to automatically deploy when code is pushed to matching branches.
          </p>
        </div>
      ) : (
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow overflow-hidden">
          <table className="min-w-full divide-y divide-gray-200 dark:divide-gray-700">
            <thead className="bg-gray-50 dark:bg-gray-700">
              <tr>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Branch Pattern</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Environment</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Project</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Tag Source</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Status</th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400 uppercase">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
              {rules.map(rule => (
                <tr key={rule.id} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                  <td className="px-6 py-4 whitespace-nowrap">
                    <code className="text-sm font-mono text-gray-900 dark:text-white bg-gray-100 dark:bg-gray-700 px-2 py-0.5 rounded">
                      {rule.branch_pattern}
                    </code>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">{getEnvBadge(rule)}</td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-600 dark:text-gray-400">
                    {rule.project_path || <span className="text-gray-400 italic">all projects</span>}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-600 dark:text-gray-400">
                    {rule.image_tag_source}
                    {rule.image_tag_source === 'regex' && rule.image_tag_regex && (
                      <code className="ml-1 text-xs text-gray-500">{rule.image_tag_regex}</code>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <button
                      onClick={() => handleToggle(rule)}
                      className={`px-2 py-0.5 text-xs font-medium rounded-full ${
                        rule.enabled
                          ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300'
                          : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400'
                      }`}
                    >
                      {rule.enabled ? 'Active' : 'Disabled'}
                    </button>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right">
                    <div className="flex items-center justify-end gap-3">
                      <button
                        onClick={() => handleEdit(rule)}
                        className="text-blue-600 hover:text-blue-800 dark:text-blue-400 text-sm"
                      >
                        Edit
                      </button>
                      <button
                        onClick={() => handleDelete(rule.id)}
                        className="text-red-600 hover:text-red-800 dark:text-red-400 text-sm"
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <div className="px-6 py-3 bg-gray-50 dark:bg-gray-700 text-sm text-gray-500 dark:text-gray-400">
            {rules.length} rule{rules.length !== 1 ? 's' : ''}
          </div>
        </div>
      )}
    </div>
  );
}
