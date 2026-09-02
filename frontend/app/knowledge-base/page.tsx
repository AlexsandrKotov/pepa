'use client';

import { useState, useEffect, useCallback } from 'react';
import { rag } from '@/lib/api';
import ConfirmModal from '@/components/ConfirmModal';

interface RAGStats {
  stats: Record<string, number>;
  total_documents: number;
  total_chunks: number;
  rag_enabled: boolean;
}

interface RAGDocument {
  id: string;
  source: string;
  source_type: string;
  source_id: string;
  content: string;
  metadata: Record<string, unknown>;
  ingested_at: string;
  expires_at: string | null;
}

interface SearchResult {
  chunk_id: string;
  document_id: string;
  content: string;
  source: string;
  source_type: string;
  score: number;
}

const SOURCE_COLORS: Record<string, string> = {
  service: '#10B981',
  entity: '#8B5CF6',
  pipeline: '#F59E0B',
  kubernetes: '#3B82F6',
  documentation: '#6366F1',
  logs: '#EF4444',
  custom: '#F472B6',
};

export default function KnowledgeBasePage() {
  const [stats, setStats] = useState<RAGStats | null>(null);
  const [documents, setDocuments] = useState<RAGDocument[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(true);
  const [searching, setSearching] = useState(false);
  const [activeTab, setActiveTab] = useState<'overview' | 'documents' | 'search'>('overview');
  const [reindexing, setReindexing] = useState(false);
  const [editDoc, setEditDoc] = useState<RAGDocument | null>(null);
  const [editContent, setEditContent] = useState('');
  const [editLoading, setEditLoading] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [createForm, setCreateForm] = useState({ title: '', source: 'custom', content: '' });
  const [createLoading, setCreateLoading] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  const loadData = useCallback(async () => {
    try {
      const [s, d] = await Promise.all([
        rag.stats(),
        rag.documents(),
      ]);
      setStats(s);
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      setDocuments((d.documents || []) as any as RAGDocument[]);
    } catch (err) {
      console.error('Failed to load knowledge base data:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadData(); }, [loadData]);

  const handleSearch = async () => {
    if (!searchQuery.trim()) return;
    setSearching(true);
    try {
      const res = await rag.search({ query: searchQuery, top_k: 10, mode: 'hybrid' });
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      setSearchResults((res.results || []) as any as SearchResult[]);
    } catch (err) {
      console.error('Search failed:', err);
    } finally {
      setSearching(false);
    }
  };

  const handleReindex = async () => {
    setReindexing(true);
    try {
      await rag.reindex();
      setTimeout(loadData, 5000);
    } catch (err) {
      console.error('Reindex failed:', err);
    } finally {
      setReindexing(false);
    }
  };

  const handleDelete = async (id: string) => {
    setDeleteConfirm(id);
  };

  const confirmDelete = async () => {
    if (!deleteConfirm) return;
    setDeleting(true);
    try {
      await rag.deleteDocument(deleteConfirm);
      setDocuments(prev => prev.filter(d => d.id !== deleteConfirm));
    } catch (err) {
      console.error('Delete failed:', err);
    }
    setDeleting(false);
    setDeleteConfirm(null);
  };

  const handleEditOpen = async (doc: RAGDocument) => {
    setEditDoc(doc);
    try {
      const full = await rag.getDocument(doc.id) as Record<string, unknown>;
      setEditContent((full.content as string) || doc.content || '');
    } catch {
      setEditContent(doc.content || '');
    }
  };

  const handleEditSave = async () => {
    if (!editDoc) return;
    setEditLoading(true);
    try {
      await rag.updateDocument(editDoc.id, { content: editContent });
      setEditDoc(null);
      setEditContent('');
      loadData();
    } catch (err) {
      console.error('Update failed:', err);
    } finally {
      setEditLoading(false);
    }
  };

  const handleCreate = async () => {
    if (!createForm.title.trim() || !createForm.content.trim()) return;
    setCreateLoading(true);
    try {
      await rag.createDocument({
        title: createForm.title,
        source: createForm.source || 'custom',
        content: createForm.content,
      });
      setShowCreate(false);
      setCreateForm({ title: '', source: 'custom', content: '' });
      loadData();
    } catch (err) {
      console.error('Create failed:', err);
    } finally {
      setCreateLoading(false);
    }
  };

  if (loading) {
    return (
      <div style={{ padding: 24, color: 'var(--text-secondary, #888)' }}>
        Loading Knowledge Base...
      </div>
    );
  }

  return (
    <div style={{ padding: 24, maxWidth: 1200, margin: '0 auto' }}>
      <div style={{ marginBottom: 24 }}>
        <h1 style={{ fontSize: 24, fontWeight: 700, color: 'var(--text-primary, #fff)', margin: 0 }}>
          Knowledge Base
        </h1>
        <p style={{ color: 'var(--text-secondary, #888)', margin: '4px 0 0', fontSize: 14 }}>
          RAG-powered knowledge base for AI context. Documents are automatically indexed and searchable.
        </p>
      </div>

      {/* Stats Cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 16, marginBottom: 24 }}>
        <StatCard label="Documents" value={stats?.total_documents ?? 0} color="#3B82F6" />
        <StatCard label="Chunks" value={stats?.total_chunks ?? 0} color="#8B5CF6" />
        <StatCard label="RAG Status" value={stats?.rag_enabled ? 'Active' : 'Disabled'} color={stats?.rag_enabled ? '#10B981' : '#EF4444'} />
        <div style={{ background: 'var(--bg-secondary, #1a1a2e)', borderRadius: 12, padding: 16 }}>
          <div style={{ fontSize: 12, color: 'var(--text-secondary, #888)', marginBottom: 4 }}>By Source</div>
          {stats?.stats && Object.entries(stats.stats)
            .filter(([k]) => !k.startsWith('_'))
            .map(([source, count]) => (
              <div key={source} style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, padding: '2px 0' }}>
                <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  <span style={{ width: 8, height: 8, borderRadius: '50%', background: SOURCE_COLORS[source] || '#666', display: 'inline-block' }} />
                  {source}
                </span>
                <span style={{ color: 'var(--text-primary, #fff)', fontWeight: 600 }}>{count}</span>
              </div>
            ))}
        </div>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        {(['overview', 'documents', 'search'] as const).map(tab => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            style={{
              padding: '8px 16px',
              borderRadius: 8,
              border: 'none',
              background: activeTab === tab ? 'var(--bg-accent, #3B82F6)' : 'var(--bg-secondary, #1a1a2e)',
              color: activeTab === tab ? '#fff' : 'var(--text-secondary, #888)',
              cursor: 'pointer',
              fontSize: 14,
              fontWeight: activeTab === tab ? 600 : 400,
            }}
          >
            {tab.charAt(0).toUpperCase() + tab.slice(1)}
          </button>
        ))}
        <div style={{ flex: 1 }} />
        <button
          onClick={() => setShowCreate(true)}
          style={{
            padding: '8px 16px',
            borderRadius: 8,
            border: 'none',
            background: '#6366F1',
            color: '#fff',
            cursor: 'pointer',
            fontSize: 14,
          }}
        >
          + Add Document
        </button>
        <button
          onClick={handleReindex}
          disabled={reindexing}
          style={{
            padding: '8px 16px',
            borderRadius: 8,
            border: 'none',
            background: reindexing ? 'var(--bg-secondary, #333)' : '#10B981',
            color: '#fff',
            cursor: reindexing ? 'not-allowed' : 'pointer',
            fontSize: 14,
          }}
        >
          {reindexing ? 'Re-indexing...' : 'Re-index All'}
        </button>
      </div>

      {/* Tab Content */}
      {activeTab === 'overview' && (
        <div style={{ background: 'var(--bg-secondary, #1a1a2e)', borderRadius: 12, padding: 24 }}>
          <h3 style={{ margin: '0 0 12px', color: 'var(--text-primary, #fff)' }}>How it works</h3>
          <p style={{ color: 'var(--text-secondary, #888)', lineHeight: 1.6, fontSize: 14 }}>
            The Knowledge Base uses RAG (Retrieval-Augmented Generation) to give the AI assistant
            context about your platform. Documents from services, entities, pipelines, and more
            are automatically chunked, embedded, and indexed for semantic search.
          </p>
          <ul style={{ color: 'var(--text-secondary, #888)', lineHeight: 2, fontSize: 14, paddingLeft: 20 }}>
            <li><strong>Auto-ingestion:</strong> Services, entities, and pipeline runs are indexed automatically</li>
            <li><strong>Custom documents:</strong> Add your own documentation, runbooks, and guides</li>
            <li><strong>Hybrid search:</strong> Combines vector similarity + keyword matching for best results</li>
            <li><strong>Event-driven:</strong> Re-indexing happens when resources change</li>
            <li><strong>Periodic refresh:</strong> Documents expire after 30 days and are re-indexed</li>
          </ul>
        </div>
      )}

      {activeTab === 'documents' && (
        <div style={{ background: 'var(--bg-secondary, #1a1a2e)', borderRadius: 12, overflow: 'hidden' }}>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 14 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid var(--border-primary, #333)' }}>
                <th style={{ padding: '12px 16px', textAlign: 'left', color: 'var(--text-secondary, #888)', fontWeight: 500 }}>Source</th>
                <th style={{ padding: '12px 16px', textAlign: 'left', color: 'var(--text-secondary, #888)', fontWeight: 500 }}>Type</th>
                <th style={{ padding: '12px 16px', textAlign: 'left', color: 'var(--text-secondary, #888)', fontWeight: 500 }}>Content Preview</th>
                <th style={{ padding: '12px 16px', textAlign: 'left', color: 'var(--text-secondary, #888)', fontWeight: 500 }}>Ingested</th>
                <th style={{ padding: '12px 16px', textAlign: 'right', color: 'var(--text-secondary, #888)', fontWeight: 500 }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {documents.length === 0 ? (
                <tr><td colSpan={5} style={{ padding: 24, textAlign: 'center', color: 'var(--text-secondary, #888)' }}>No documents indexed yet. Click &quot;Re-index All&quot; or &quot;+ Add Document&quot; to start.</td></tr>
              ) : documents.map(doc => (
                <tr key={doc.id} style={{ borderBottom: '1px solid var(--border-primary, #222)' }}>
                  <td style={{ padding: '10px 16px' }}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                      <span style={{ width: 8, height: 8, borderRadius: '50%', background: SOURCE_COLORS[doc.source] || '#666', display: 'inline-block' }} />
                      {doc.source}
                    </span>
                  </td>
                  <td style={{ padding: '10px 16px', color: 'var(--text-secondary, #888)' }}>{doc.source_type || '-'}</td>
                  <td style={{ padding: '10px 16px', color: 'var(--text-secondary, #888)', maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {doc.content?.slice(0, 100) || '-'}
                  </td>
                  <td style={{ padding: '10px 16px', color: 'var(--text-secondary, #888)', fontSize: 12 }}>
                    {doc.ingested_at ? new Date(doc.ingested_at).toLocaleDateString() : '-'}
                  </td>
                  <td style={{ padding: '10px 16px', textAlign: 'right' }}>
                    <button onClick={() => handleEditOpen(doc)} style={{ background: 'none', border: 'none', color: '#3B82F6', cursor: 'pointer', fontSize: 13, marginRight: 8 }}>Edit</button>
                    <button onClick={() => handleDelete(doc.id)} style={{ background: 'none', border: 'none', color: '#EF4444', cursor: 'pointer', fontSize: 13 }}>Delete</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {activeTab === 'search' && (
        <div>
          <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
            <input
              type="text"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
              placeholder="Search the knowledge base..."
              style={{
                flex: 1,
                padding: '10px 16px',
                borderRadius: 8,
                border: '1px solid var(--border-primary, #333)',
                background: 'var(--bg-secondary, #1a1a2e)',
                color: 'var(--text-primary, #fff)',
                fontSize: 14,
                outline: 'none',
              }}
            />
            <button
              onClick={handleSearch}
              disabled={searching}
              style={{
                padding: '10px 20px',
                borderRadius: 8,
                border: 'none',
                background: '#3B82F6',
                color: '#fff',
                cursor: searching ? 'not-allowed' : 'pointer',
                fontSize: 14,
              }}
            >
              {searching ? 'Searching...' : 'Search'}
            </button>
          </div>

          {searchResults.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {searchResults.map((r, i) => (
                <div key={r.chunk_id || i} style={{ background: 'var(--bg-secondary, #1a1a2e)', borderRadius: 12, padding: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                    <span style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                      <span style={{ width: 8, height: 8, borderRadius: '50%', background: SOURCE_COLORS[r.source] || '#666', display: 'inline-block' }} />
                      <span style={{ fontWeight: 600, color: 'var(--text-primary, #fff)', fontSize: 14 }}>{r.source}/{r.source_type}</span>
                    </span>
                    <span style={{ fontSize: 12, color: 'var(--text-secondary, #888)' }}>score: {r.score?.toFixed(3)}</span>
                  </div>
                  <pre style={{ fontSize: 13, color: 'var(--text-secondary, #ccc)', whiteSpace: 'pre-wrap', margin: 0, fontFamily: 'inherit', lineHeight: 1.5 }}>
                    {r.content}
                  </pre>
                </div>
              ))}
            </div>
          )}

          {searchResults.length === 0 && searchQuery && !searching && (
            <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-secondary, #888)' }}>
              No results found. Try a different query.
            </div>
          )}
        </div>
      )}

      {/* Edit Document Modal */}
      {editDoc && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div style={{ background: 'var(--bg-primary, #0f0f1a)', borderRadius: 16, padding: 24, width: '90%', maxWidth: 800, maxHeight: '80vh', display: 'flex', flexDirection: 'column' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h2 style={{ margin: 0, color: 'var(--text-primary, #fff)', fontSize: 18 }}>
                Edit Document
                <span style={{ fontSize: 12, color: 'var(--text-secondary, #888)', marginLeft: 8, fontWeight: 400 }}>
                  {editDoc.source}/{editDoc.source_type}
                </span>
              </h2>
              <button onClick={() => setEditDoc(null)} style={{ background: 'none', border: 'none', color: 'var(--text-secondary, #888)', cursor: 'pointer', fontSize: 20 }}>&times;</button>
            </div>
            <textarea
              value={editContent}
              onChange={e => setEditContent(e.target.value)}
              style={{
                flex: 1,
                minHeight: 300,
                padding: 16,
                borderRadius: 8,
                border: '1px solid var(--border-primary, #333)',
                background: 'var(--bg-secondary, #1a1a2e)',
                color: 'var(--text-primary, #fff)',
                fontSize: 14,
                fontFamily: 'monospace',
                lineHeight: 1.6,
                resize: 'vertical',
                outline: 'none',
              }}
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
              <button onClick={() => setEditDoc(null)} style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--border-primary, #333)', background: 'transparent', color: 'var(--text-secondary, #888)', cursor: 'pointer', fontSize: 14 }}>Cancel</button>
              <button onClick={handleEditSave} disabled={editLoading} style={{ padding: '8px 16px', borderRadius: 8, border: 'none', background: '#3B82F6', color: '#fff', cursor: editLoading ? 'not-allowed' : 'pointer', fontSize: 14 }}>
                {editLoading ? 'Saving...' : 'Save & Re-index'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Create Document Modal */}
      {showCreate && (
        <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
          <div style={{ background: 'var(--bg-primary, #0f0f1a)', borderRadius: 16, padding: 24, width: '90%', maxWidth: 800, maxHeight: '80vh', display: 'flex', flexDirection: 'column' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <h2 style={{ margin: 0, color: 'var(--text-primary, #fff)', fontSize: 18 }}>Add Document</h2>
              <button onClick={() => setShowCreate(false)} style={{ background: 'none', border: 'none', color: 'var(--text-secondary, #888)', cursor: 'pointer', fontSize: 20 }}>&times;</button>
            </div>
            <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
              <input
                type="text"
                value={createForm.title}
                onChange={e => setCreateForm(f => ({ ...f, title: e.target.value }))}
                placeholder="Document title"
                style={{
                  flex: 2,
                  padding: '10px 16px',
                  borderRadius: 8,
                  border: '1px solid var(--border-primary, #333)',
                  background: 'var(--bg-secondary, #1a1a2e)',
                  color: 'var(--text-primary, #fff)',
                  fontSize: 14,
                  outline: 'none',
                }}
              />
              <select
                value={createForm.source}
                onChange={e => setCreateForm(f => ({ ...f, source: e.target.value }))}
                style={{
                  flex: 1,
                  padding: '10px 16px',
                  borderRadius: 8,
                  border: '1px solid var(--border-primary, #333)',
                  background: 'var(--bg-secondary, #1a1a2e)',
                  color: 'var(--text-primary, #fff)',
                  fontSize: 14,
                  outline: 'none',
                }}
              >
                <option value="custom">Custom</option>
                <option value="documentation">Documentation</option>
                <option value="runbook">Runbook</option>
                <option value="guide">Guide</option>
              </select>
            </div>
            <textarea
              value={createForm.content}
              onChange={e => setCreateForm(f => ({ ...f, content: e.target.value }))}
              placeholder="Document content (Markdown supported)..."
              style={{
                flex: 1,
                minHeight: 300,
                padding: 16,
                borderRadius: 8,
                border: '1px solid var(--border-primary, #333)',
                background: 'var(--bg-secondary, #1a1a2e)',
                color: 'var(--text-primary, #fff)',
                fontSize: 14,
                fontFamily: 'monospace',
                lineHeight: 1.6,
                resize: 'vertical',
                outline: 'none',
              }}
            />
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
              <button onClick={() => setShowCreate(false)} style={{ padding: '8px 16px', borderRadius: 8, border: '1px solid var(--border-primary, #333)', background: 'transparent', color: 'var(--text-secondary, #888)', cursor: 'pointer', fontSize: 14 }}>Cancel</button>
              <button onClick={handleCreate} disabled={createLoading || !createForm.title.trim() || !createForm.content.trim()} style={{ padding: '8px 16px', borderRadius: 8, border: 'none', background: '#6366F1', color: '#fff', cursor: (createLoading || !createForm.title.trim() || !createForm.content.trim()) ? 'not-allowed' : 'pointer', fontSize: 14 }}>
                {createLoading ? 'Creating...' : 'Create & Index'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete Confirmation */}
      <ConfirmModal
        open={!!deleteConfirm}
        title="Delete this document?"
        description="This document and all its chunks will be permanently deleted. This action cannot be undone."
        confirmLabel="Delete"
        variant="danger"
        loading={deleting}
        onConfirm={confirmDelete}
        onCancel={() => setDeleteConfirm(null)}
      />
    </div>
  );
}

function StatCard({ label, value, color }: { label: string; value: string | number; color: string }) {
  return (
    <div style={{ background: 'var(--bg-secondary, #1a1a2e)', borderRadius: 12, padding: 16 }}>
      <div style={{ fontSize: 12, color: 'var(--text-secondary, #888)', marginBottom: 4 }}>{label}</div>
      <div style={{ fontSize: 24, fontWeight: 700, color }}>{value}</div>
    </div>
  );
}
