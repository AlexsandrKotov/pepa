'use client';

import { useState, useCallback, useEffect } from 'react';
import ConfirmModal from '@/components/ConfirmModal';
import { connections } from '@/lib/api';
import { friendlyError } from '@/lib/errors';

interface JenkinsGroovyEditorProps {
  connectionId: string;
  jobName: string;
  /** Called after a successful save or create */
  onSaved?: () => void;
}

const DECLARATIVE_TEMPLATE = `pipeline {
    agent any
    stages {
        stage('Build') {
            steps {
                echo 'Building...'
            }
        }
        stage('Test') {
            steps {
                echo 'Testing...'
            }
        }
        stage('Deploy') {
            steps {
                echo 'Deploying...'
            }
        }
    }
}`;

const SCRIPTED_TEMPLATE = `node {
    stage('Build') {
        echo 'Building...'
    }
    stage('Test') {
        echo 'Testing...'
    }
    stage('Deploy') {
        echo 'Deploying...'
    }
}`;

type Mode = 'view' | 'edit' | 'create';

export default function JenkinsGroovyEditor({ connectionId, jobName, onSaved }: JenkinsGroovyEditorProps) {
  const [mode, setMode] = useState<Mode>('view');
  const [script, setScript] = useState('');
  const [savedScript, setSavedScript] = useState('');
  const [pipelineType, setPipelineType] = useState<'cps' | 'cps-scm' | ''>('');
  const [newJobName, setNewJobName] = useState('');
  const [newPipelineType, setNewPipelineType] = useState<'declarative' | 'scripted'>('declarative');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [showSaveConfirm, setShowSaveConfirm] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [justCreated, setJustCreated] = useState(false);

  // Fetch the pipeline script
  const fetchScript = useCallback(async () => {
    if (!jobName) return;
    setLoading(true);
    setError('');
    try {
      const { data } = await connections.execute(connectionId, 'get_pipeline_script', { job_name: jobName });
      const scriptData = data as { script?: string; type?: 'cps' | 'cps-scm' | '' };
      setScript(scriptData.script || '');
      setSavedScript(scriptData.script || '');
      setPipelineType(scriptData.type || '');
      setDirty(false);
    } catch (err) {
      setError(friendlyError(err).message);
    } finally {
      setLoading(false);
    }
  }, [connectionId, jobName]);

  useEffect(() => { void fetchScript(); }, [fetchScript]);

  // Reset the post-create flag when the parent navigates to a different job.
  useEffect(() => { setJustCreated(false); }, [jobName]);

  const cancelEdit = () => {
    setMode('view');
    setScript(savedScript);
    setDirty(false);
    setError('');
  };

  // Save the pipeline script
  const saveScript = async () => {
    setLoading(true);
    setError('');
    setSuccess('');
    try {
      await connections.execute(connectionId, 'update_pipeline_script', { job_name: jobName, script });
      setSavedScript(script);
      setSuccess('Pipeline script saved successfully');
      setDirty(false);
      setMode('view');
      onSaved?.();
    } catch (err) {
      setError(friendlyError(err).message);
    } finally {
      setLoading(false);
      setShowSaveConfirm(false);
    }
  };

  // Create a new pipeline job
  const createJob = async () => {
    if (!newJobName) {
      setError('Job name is required');
      return;
    }
    setLoading(true);
    setError('');
    setSuccess('');
    try {
      const parts = newJobName.trim().split('/');
      const name = parts.pop();
      await connections.execute(connectionId, 'create_job', {
        job_name: name, folder: parts.join('/'), script, sandbox: true,
      });
      setSuccess(`Pipeline job "${newJobName}" created successfully`);
      // Keep the script the user authored in the editor so they can review or
      // save it again. The job list above will refresh via onSaved and the
      // user can navigate to the new job to load it from Jenkins.
      setSavedScript(script);
      setDirty(false);
      setMode('view');
      setJustCreated(true);
      onSaved?.();
    } catch (err) {
      setError(friendlyError(err).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Toolbar */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {mode === 'view' && (
            <>
              <span className="text-sm font-medium text-[var(--text-primary)]">
                {jobName || 'Pipeline Script'}
              </span>
              {pipelineType && (
                <span className="text-xs px-2 py-0.5 rounded-full bg-blue-500/10 text-blue-500 border border-blue-500/20">
                  {pipelineType === 'cps-scm' ? 'SCM-based' : 'In-Jenkins'}
                </span>
              )}
            </>
          )}
          {mode === 'edit' && (
            <span className="text-sm font-medium text-[var(--text-primary)]">
              Editing: {jobName}
            </span>
          )}
          {mode === 'create' && (
            <span className="text-sm font-medium text-[var(--text-primary)]">
              New Pipeline Job
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {mode === 'view' && (
            <>
              <button
                onClick={() => setMode('edit')}
                disabled={loading || !jobName || pipelineType !== 'cps'}
                className="px-3 py-1.5 text-sm bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 transition-colors"
              >
                Edit
              </button>
              <button
                onClick={() => { setMode('create'); setNewPipelineType('declarative'); setScript(DECLARATIVE_TEMPLATE); setError(''); setSuccess(''); }}
                disabled={loading}
                className="px-3 py-1.5 text-sm bg-emerald-500 text-white rounded-lg hover:bg-emerald-600 transition-colors"
              >
                New Pipeline
              </button>
            </>
          )}
          {mode === 'edit' && (
            <>
              <button
                onClick={cancelEdit}
                disabled={loading}
                className="px-3 py-1.5 text-sm text-[var(--text-secondary)] border border-[var(--border)] rounded-lg hover:bg-[var(--surface-hover)] transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={() => setShowSaveConfirm(true)}
                disabled={!dirty || loading}
                className="px-3 py-1.5 text-sm bg-blue-500 text-white rounded-lg hover:bg-blue-600 disabled:opacity-50 transition-colors"
              >
                {loading ? 'Saving...' : 'Save'}
              </button>
            </>
          )}
          {mode === 'create' && (
            <>
              <button
                onClick={cancelEdit}
                disabled={loading}
                className="px-3 py-1.5 text-sm text-[var(--text-secondary)] border border-[var(--border)] rounded-lg hover:bg-[var(--surface-hover)] transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={createJob}
                disabled={loading || !newJobName}
                className="px-3 py-1.5 text-sm bg-emerald-500 text-white rounded-lg hover:bg-emerald-600 disabled:opacity-50 transition-colors"
              >
                {loading ? 'Creating...' : 'Create'}
              </button>
            </>
          )}
        </div>
      </div>

      {/* Create mode: job name + type selector */}
      {mode === 'create' && (
        <div className="space-y-3">
          <div>
            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Job Name *</label>
            <input
              type="text"
              value={newJobName}
              onChange={e => setNewJobName(e.target.value)}
              placeholder="my-pipeline (use / for folders: folder/my-pipeline)"
              className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-[var(--text-primary)] mb-1">Pipeline Type</label>
            <div className="flex gap-3">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  checked={newPipelineType === 'declarative'}
                  onChange={() => { setNewPipelineType('declarative'); setScript(DECLARATIVE_TEMPLATE); }}
                  className="text-[var(--accent)]"
                />
                <span className="text-sm">Declarative</span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  checked={newPipelineType === 'scripted'}
                  onChange={() => { setNewPipelineType('scripted'); setScript(SCRIPTED_TEMPLATE); }}
                  className="text-[var(--accent)]"
                />
                <span className="text-sm">Scripted</span>
              </label>
            </div>
          </div>
        </div>
      )}

      {/* Feedback */}
      {error && (
        <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-lg text-sm text-red-500">
          {error}
        </div>
      )}
      {success && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-lg text-sm text-emerald-500">
          {success}
        </div>
      )}

      {/* Editor / Viewer */}
      <div className="relative">
        {mode === 'view' && pipelineType === 'cps-scm' && (
          <p className="p-3 text-sm text-[var(--text-secondary)]">This pipeline uses a Jenkinsfile from source control. Edit it in its Git repository.</p>
        )}
        {mode === 'view' && jobName && !script && !loading && pipelineType !== 'cps-scm' && (
          <button
            onClick={fetchScript}
            className="w-full py-8 text-sm text-[var(--text-tertiary)] border border-dashed border-[var(--border)] rounded-xl hover:border-blue-400 hover:text-blue-500 transition-colors"
          >
            Click to load pipeline script
          </button>
        )}
        {(mode !== 'view' || script || loading || justCreated) && (
          <textarea
            value={script}
            onChange={e => { setScript(e.target.value); setDirty(true); }}
            readOnly={mode === 'view'}
            spellCheck={false}
            className={`w-full h-96 px-4 py-3 font-mono text-sm leading-relaxed border border-[var(--border)] rounded-xl resize-y focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent ${
              mode === 'view'
                ? 'bg-[var(--surface-hover)] text-[var(--text-primary)] cursor-default'
                : 'bg-[var(--surface)] text-[var(--text-primary)]'
            }`}
            placeholder={loading ? 'Loading...' : '// Groovy pipeline script will appear here'}
          />
        )}
      </div>

      {/* Line count */}
      {script && (
        <div className="text-xs text-[var(--text-tertiary)]">
          {script.split('\n').length} lines
          {dirty && <span className="ml-2 text-amber-500">• unsaved changes</span>}
        </div>
      )}

      {/* Save confirmation modal */}
      {showSaveConfirm && (
        <ConfirmModal
          open={showSaveConfirm}
          title="Save Pipeline Script"
          description="This will update the Groovy pipeline script for this Jenkins job. The next build will use the new script. Are you sure?"
          confirmLabel="Save"
          onConfirm={saveScript}
          onCancel={() => setShowSaveConfirm(false)}
        />
      )}
    </div>
  );
}
