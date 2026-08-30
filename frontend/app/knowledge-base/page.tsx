'use client';

import { useState, useEffect, useCallback } from 'react';
import { rag } from '@/lib/api';

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
      setTimeout(loadData, 5000); // Wait for background reindex
    } catch (err) {
      console.error('Reindex failed:', err);
    } finally {
      setReindexing(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this document and its chunks?')) return;
    try {
      await rag.deleteDocument(id);
      setDocuments(prev => prev.filter(d => d.id !== id));
    } catch (err) {
      console.error('Delete failed:', err);
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
                <tr><td colSpan={5} style={{ padding: 24, textAlign: 'center', color: 'var(--text-secondary, #888)' }}>No documents indexed yet. Click &quot;Re-index All&quot; to start.</td></tr>
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
