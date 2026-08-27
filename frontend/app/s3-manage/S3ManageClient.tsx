'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { connections as connectionsAPI, s3Browser, authHeaders, type Connection, type S3Bucket, type S3Object, type S3CredentialStatus } from '@/lib/api';
import { friendlyError } from '@/lib/errors';
import BrandIcon from '@/components/BrandIcon';
import ConfirmModal from '@/components/ConfirmModal';

// ── Helpers ────────────────────────────────────────────────────
function formatSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatDate(iso: string): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

function fileIcon(key: string): string {
  const ext = key.split('.').pop()?.toLowerCase() || '';
  if (['json', 'yaml', 'yml', 'toml', 'xml'].includes(ext)) return '{ }';
  if (['txt', 'md', 'log', 'csv'].includes(ext)) return 'TXT';
  if (['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)) return 'IMG';
  if (['zip', 'tar', 'gz', 'bz2'].includes(ext)) return 'ZIP';
  if (['pdf'].includes(ext)) return 'PDF';
  if (['go', 'py', 'js', 'ts', 'sh', 'sql'].includes(ext)) return 'SRC';
  return 'FILE';
}

const TEXT_EXTS = ['json', 'yaml', 'yml', 'toml', 'xml', 'txt', 'md', 'log', 'csv', 'go', 'py', 'js', 'ts', 'sh', 'sql', 'env', 'cfg', 'conf', 'ini', 'properties', 'html', 'htm', 'bash'];
const IMAGE_EXTS = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'];

function previewType(key: string): 'text' | 'image' | 'pdf' | null {
  const ext = key.split('.').pop()?.toLowerCase() || '';
  if (TEXT_EXTS.includes(ext)) return 'text';
  if (IMAGE_EXTS.includes(ext)) return 'image';
  if (ext === 'pdf') return 'pdf';
  return null;
}

// ── Main component ─────────────────────────────────────────────
export default function S3ManageClient({
  initialConnectionId,
  initialBucket,
  initialPrefix,
}: {
  initialConnectionId?: string;
  initialBucket?: string;
  initialPrefix?: string;
}) {
  const router = useRouter();
  const searchParams = useSearchParams();

  // Connection list state
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);

  // Bucket list state
  const [buckets, setBuckets] = useState<S3Bucket[]>([]);
  const [loadingBuckets, setLoadingBuckets] = useState(false);

  // Object browser state
  const [objects, setObjects] = useState<S3Object[]>([]);
  const [folders, setFolders] = useState<{ name: string }[]>([]);
  const [loadingObjects, setLoadingObjects] = useState(false);
  const [currentPrefix, setCurrentPrefix] = useState(initialPrefix || '');

  // UI state
  const [feedback, setFeedback] = useState<{ ok: boolean; text: string } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<{ type: 'object' | 'bucket'; key: string; bucket?: string } | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [showCreateBucket, setShowCreateBucket] = useState(false);
  const [newBucketName, setNewBucketName] = useState('');
  const [creatingBucket, setCreatingBucket] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<{ done: number; total: number } | null>(null);
  const [selectedObjects, setSelectedObjects] = useState<Set<string>>(new Set());
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);

  // Preview state
  const [previewKey, setPreviewKey] = useState<string | null>(null);
  const [previewContent, setPreviewContent] = useState<string>('');
  const [previewContentType, setPreviewContentType] = useState<string>('');
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [previewBlobUrl, setPreviewBlobUrl] = useState<string | null>(null);

  // Credential status
  const [credStatus, setCredStatus] = useState<S3CredentialStatus | null>(null);

  const activeConnection = initialConnectionId || searchParams.get('connection') || '';
  const activeBucket = initialBucket || searchParams.get('bucket') || '';

  // ── Load connections ──────────────────────────────────────────
  useEffect(() => {
    connectionsAPI.list('storage')
      .then(data => setConnections((data.connections || []).filter(c => c.type === 'storage')))
      .catch(() => setConnections([]))
      .finally(() => setLoading(false));
  }, []);

  // ── Load buckets when connection changes ──────────────────────
  const loadBuckets = useCallback(async (connId: string) => {
    setLoadingBuckets(true);
    try {
      const data = await s3Browser.listBuckets(connId);
      setBuckets(data.buckets || []);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Failed to load buckets: ${fe.message}` });
      setBuckets([]);
    } finally {
      setLoadingBuckets(false);
    }
  }, []);

  useEffect(() => {
    if (activeConnection) {
      loadBuckets(activeConnection);
      // Fetch credential status for this connection
      s3Browser.getCredentialStatus(activeConnection)
        .then(setCredStatus)
        .catch(() => setCredStatus(null));
    } else {
      setCredStatus(null);
    }
  }, [activeConnection, loadBuckets]);

  // ── Load objects when bucket/prefix changes ───────────────────
  const loadObjects = useCallback(async (connId: string, bucket: string, prefix: string) => {
    setLoadingObjects(true);
    try {
      const data = await s3Browser.listObjects(connId, bucket, prefix || undefined);
      setObjects(data.objects || []);
      setFolders(data.folders || []);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Failed to list objects: ${fe.message}` });
      setObjects([]);
    } finally {
      setLoadingObjects(false);
    }
  }, []);

  useEffect(() => {
    if (activeConnection && activeBucket) {
      loadObjects(activeConnection, activeBucket, currentPrefix);
    }
  }, [activeConnection, activeBucket, currentPrefix, loadObjects]);

  // ── Navigation helpers ────────────────────────────────────────
  const navigateTo = (params: { connection?: string; bucket?: string; prefix?: string }) => {
    const sp = new URLSearchParams();
    if (params.connection) sp.set('connection', params.connection);
    if (params.bucket) sp.set('bucket', params.bucket);
    if (params.prefix) sp.set('prefix', params.prefix);
    setCurrentPrefix(params.prefix || '');
    setSelectedObjects(new Set());
    router.push(`/s3-manage?${sp.toString()}`);
  };

  // ── Breadcrumb parts ──────────────────────────────────────────
  const breadcrumbParts = (): { label: string; prefix: string }[] => {
    if (!currentPrefix) return [];
    const parts = currentPrefix.split('/').filter(Boolean);
    return parts.map((p, i) => ({
      label: p,
      prefix: parts.slice(0, i + 1).join('/') + '/',
    }));
  };

  // ── Upload handler (multi-file, preserves relative paths for folders) ──
  const handleUpload = async (files: FileList | null, isFolder = false) => {
    if (!files || files.length === 0 || !activeConnection || !activeBucket) return;
    setUploading(true);
    setUploadProgress({ done: 0, total: files.length });
    try {
      const fileArr = Array.from(files);
      const relativePaths = isFolder
        ? fileArr.map(f => (f as unknown as { webkitRelativePath: string }).webkitRelativePath || f.name)
        : undefined;

      const result = await s3Browser.uploadFiles(activeConnection, activeBucket, fileArr, currentPrefix, relativePaths);
      if (result.failed > 0) {
        setFeedback({ ok: false, text: `Uploaded ${result.uploaded} of ${result.total} file(s), ${result.failed} failed` });
      } else {
        setFeedback({ ok: true, text: `Uploaded ${result.uploaded} file(s) successfully` });
      }
      loadObjects(activeConnection, activeBucket, currentPrefix);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Upload failed: ${fe.message}` });
    } finally {
      setUploading(false);
      setUploadProgress(null);
      if (fileInputRef.current) fileInputRef.current.value = '';
      if (folderInputRef.current) folderInputRef.current.value = '';
    }
  };

  // ── Delete handler ────────────────────────────────────────────
  const handleDelete = async () => {
    if (!deleteTarget || !activeConnection) return;
    setDeleting(true);
    try {
      if (deleteTarget.type === 'object' && deleteTarget.bucket) {
        await s3Browser.deleteObject(activeConnection, deleteTarget.bucket, deleteTarget.key);
        setFeedback({ ok: true, text: `Deleted: ${deleteTarget.key}` });
        loadObjects(activeConnection, deleteTarget.bucket, currentPrefix);
      } else if (deleteTarget.type === 'bucket') {
        await s3Browser.deleteBucket(activeConnection, deleteTarget.key);
        setFeedback({ ok: true, text: `Bucket deleted: ${deleteTarget.key}` });
        loadBuckets(activeConnection);
      }
      setDeleteTarget(null);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Delete failed: ${fe.message}` });
      setDeleteTarget(null);
    } finally {
      setDeleting(false);
    }
  };

  // ── Batch delete ──────────────────────────────────────────────
  const handleBatchDelete = async () => {
    if (!activeConnection || !activeBucket || selectedObjects.size === 0) return;
    setDeleting(true);
    try {
      for (const key of selectedObjects) {
        await s3Browser.deleteObject(activeConnection, activeBucket, key);
      }
      setFeedback({ ok: true, text: `Deleted ${selectedObjects.size} object(s)` });
      setSelectedObjects(new Set());
      loadObjects(activeConnection, activeBucket, currentPrefix);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Batch delete failed: ${fe.message}` });
    } finally {
      setDeleting(false);
    }
  };

  // ── Preview handler ─────────────────────────────────────────────
  const handlePreview = async (key: string) => {
    if (!activeConnection || !activeBucket) return;
    setPreviewKey(key);
    setPreviewLoading(true);
    setPreviewError(null);
    setPreviewContent('');
    setPreviewContentType('');
    setPreviewBlobUrl(null);
    try {
      const pType = previewType(key);
      if (pType === 'image' || pType === 'pdf') {
        // For images and PDFs, fetch as blob to create an authenticated object URL
        const url = s3Browser.getPreviewUrl(activeConnection, activeBucket, key);
        const res = await fetch(url, { credentials: 'include', headers: authHeaders() });
        if (!res.ok) {
          const err = await res.json().catch(() => ({ error: 'preview failed' }));
          throw new Error(err.error || 'preview failed');
        }
        const blob = await res.blob();
        const blobUrl = URL.createObjectURL(blob);
        setPreviewBlobUrl(blobUrl);
        setPreviewContentType(blob.type || 'application/octet-stream');
      } else {
        // For text files, fetch as text
        const data = await s3Browser.previewFile(activeConnection, activeBucket, key);
        setPreviewContent(data.content);
        setPreviewContentType(data.contentType);
      }
    } catch (err) {
      setPreviewError(friendlyError(err).message);
    } finally {
      setPreviewLoading(false);
    }
  };

  const closePreview = () => {
    if (previewBlobUrl) URL.revokeObjectURL(previewBlobUrl);
    setPreviewKey(null);
    setPreviewContent('');
    setPreviewContentType('');
    setPreviewError(null);
    setPreviewBlobUrl(null);
  };

  // ── Create bucket handler ─────────────────────────────────────
  const handleCreateBucket = async () => {
    if (!activeConnection || !newBucketName) return;
    setCreatingBucket(true);
    try {
      await s3Browser.createBucket(activeConnection, newBucketName);
      setFeedback({ ok: true, text: `Bucket "${newBucketName}" created` });
      setShowCreateBucket(false);
      setNewBucketName('');
      loadBuckets(activeConnection);
    } catch (err) {
      const fe = friendlyError(err);
      setFeedback({ ok: false, text: `Create bucket failed: ${fe.message}` });
    } finally {
      setCreatingBucket(false);
    }
  };

  // ── Toggle object selection ───────────────────────────────────
  const toggleSelect = (key: string) => {
    setSelectedObjects(prev => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  };

  // ── Drag & drop ───────────────────────────────────────────────
  const [isDragging, setIsDragging] = useState(false);
  const handleDragOver = (e: React.DragEvent) => { e.preventDefault(); setIsDragging(true); };
  const handleDragLeave = () => setIsDragging(false);
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
    // Check if dropped items include directories
    const items = e.dataTransfer.items;
    if (items && items.length > 0 && typeof items[0].webkitGetAsEntry === 'function') {
      const hasDirectory = Array.from(items).some(item => {
        const entry = item.webkitGetAsEntry();
        return entry && entry.isDirectory;
      });
      if (hasDirectory) {
        // For directory drops, use the folder upload path
        handleUpload(e.dataTransfer.files, true);
        return;
      }
    }
    handleUpload(e.dataTransfer.files, false);
  };

  // ── Active connection info ────────────────────────────────────
  const activeConn = connections.find(c => c.id === activeConnection);

  // ══════════════════════════════════════════════════════════════
  // RENDER
  // ══════════════════════════════════════════════════════════════
  return (
    <div className="-mx-6 -my-6 min-h-full page-mesh-bg">
      <div className="px-6 py-6 space-y-6">

        {/* ── Header ─────────────────────────────────────────── */}
        <div className="page-animate">
          <div className="flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2">
                <h1 className="page-title-modern">S3 Manage</h1>
              </div>
              <p className="page-subtitle-modern">Browse and manage S3-compatible object storage</p>
            </div>
            {activeConnection && (
              <button
                onClick={() => navigateTo({})}
                className="btn btn-secondary"
              >
                ← All Connections
              </button>
            )}
          </div>
        </div>

        {/* ── Feedback ───────────────────────────────────────── */}
        {feedback && (
          <div className={`page-animate-up rounded-xl border p-4 flex items-start justify-between gap-3 ${
            feedback.ok ? 'bg-emerald-500/10 border-emerald-500/20' : 'bg-red-500/10 border-red-500/20'
          }`}>
            <p className={`text-sm font-medium ${feedback.ok ? 'text-emerald-600' : 'text-red-500'}`}>
              {feedback.ok ? '✓ ' : '⚠ '}{feedback.text}
            </p>
            <button onClick={() => setFeedback(null)} className="text-xs text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] shrink-0">✕</button>
          </div>
        )}

        {/* ── Connection List ────────────────────────────────── */}
        {!activeConnection && (
          <>
            {loading ? (
              <div className="card card-body text-center py-12">
                <div className="flex items-center justify-center gap-2 text-[var(--text-tertiary)]">
                  <svg className="animate-spin w-4 h-4" viewBox="0 0 24 24" fill="none">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                  </svg>
                  <p className="text-[13px]">Loading connections...</p>
                </div>
              </div>
            ) : connections.length === 0 ? (
              <div className="page-animate-up text-center py-20 card" style={{ borderRadius: '16px' }}>
                <div className="mb-4 opacity-20"><BrandIcon name="storage" size={48} /></div>
                <h3 className="text-lg font-semibold text-[var(--text-primary)] mb-2">No S3 connections</h3>
                <p className="text-[var(--text-secondary)] mb-6">Add an S3-compatible storage connection to browse and manage files</p>
                <Link href="/connections?type=storage" className="btn btn-primary">
                  + Add S3 Connection
                </Link>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {connections.map(conn => (
                  <button
                    key={conn.id}
                    onClick={() => navigateTo({ connection: conn.id })}
                    className="card card-body modern-card-hover hover:border-[var(--text-tertiary)] text-left"
                    style={{ borderRadius: '12px' }}
                  >
                    <div className="flex items-start justify-between mb-3">
                      <div className="flex items-center gap-3">
                        <div className="w-10 h-10 rounded-xl flex items-center justify-center" style={{ background: 'linear-gradient(135deg, rgba(245,158,11,0.1), rgba(245,158,11,0.05))' }}>
                          <BrandIcon name="storage" size={22} style={{ color: '#F59E0B' }} />
                        </div>
                        <div>
                          <h3 className="font-semibold text-[var(--text-primary)]">{conn.name}</h3>
                          <p className="text-xs text-[var(--text-tertiary)]">{conn.config?.endpoint as string || 'No endpoint'}</p>
                        </div>
                      </div>
                      <span className={`text-xs px-2 py-1 rounded-full border ${
                        conn.status === 'connected' ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/20' : 'bg-[var(--border-light)] text-[var(--text-tertiary)] border-[var(--border)]'
                      }`}>
                        {conn.status}
                      </span>
                    </div>
                    {conn.description && (
                      <p className="text-sm text-[var(--text-secondary)] line-clamp-2">{conn.description}</p>
                    )}
                    <div className="flex items-center justify-between pt-3 border-t border-[var(--border-light)] mt-3">
                      <span className="text-xs text-[var(--text-tertiary)]">Click to browse buckets</span>
                      <svg className="w-4 h-4 text-[var(--text-tertiary)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                      </svg>
                    </div>
                  </button>
                ))}
              </div>
            )}
          </>
        )}

        {/* ── Bucket List ────────────────────────────────────── */}
        {activeConnection && !activeBucket && (
          <div className="space-y-4">
            {/* Connection info bar */}
            {activeConn && (
              <div className="card card-body flex items-center justify-between flex-wrap gap-3">
                <div className="flex items-center gap-3">
                  <div className="w-9 h-9 rounded-xl flex items-center justify-center" style={{ background: 'linear-gradient(135deg, rgba(245,158,11,0.1), rgba(245,158,11,0.05))' }}>
                    <BrandIcon name="storage" size={20} style={{ color: '#F59E0B' }} />
                  </div>
                  <div>
                    <h2 className="font-semibold text-[var(--text-primary)]">{activeConn.name}</h2>
                    <p className="text-xs text-[var(--text-tertiary)]">{activeConn.config?.endpoint as string}</p>
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  {credStatus && (
                    credStatus.source === 'user' ? (
                      <span className="text-xs bg-emerald-500/10 text-emerald-600 border border-emerald-500/20 px-2.5 py-1 rounded-full">
                        Your credentials ({credStatus.username})
                      </span>
                    ) : (
                      <Link href="/credentials" className="text-xs bg-amber-500/10 text-amber-600 border border-amber-500/20 px-2.5 py-1 rounded-full hover:bg-amber-500/20 transition-colors">
                        Admin credentials — add your own
                      </Link>
                    )
                  )}
                  <button
                    onClick={() => setShowCreateBucket(true)}
                    className="btn btn-primary text-sm"
                  >
                    + Create Bucket
                  </button>
                </div>
              </div>
            )}

            {loadingBuckets ? (
              <div className="card card-body text-center py-12">
                <p className="text-[13px] text-[var(--text-tertiary)]">Loading buckets...</p>
              </div>
            ) : buckets.length === 0 ? (
              <div className="card text-center py-12" style={{ borderRadius: '12px' }}>
                <p className="text-[var(--text-secondary)] mb-4">No buckets found</p>
                <button onClick={() => setShowCreateBucket(true)} className="btn btn-primary text-sm">
                  + Create Bucket
                </button>
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {buckets.map(b => (
                  <div
                    key={b.name}
                    className="card card-body group hover:border-[var(--text-tertiary)]"
                    style={{ borderRadius: '12px' }}
                  >
                    <div className="flex items-start justify-between">
                      <button
                        onClick={() => navigateTo({ connection: activeConnection, bucket: b.name })}
                        className="flex items-center gap-3 flex-1 min-w-0 text-left"
                      >
                        <svg className="w-8 h-8 text-amber-500/60 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5M10 11.25h4M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" />
                        </svg>
                        <div className="min-w-0">
                          <h3 className="font-semibold text-[var(--text-primary)] truncate">{b.name}</h3>
                          <p className="text-xs text-[var(--text-tertiary)]">{formatDate(b.creation_date)}</p>
                        </div>
                      </button>
                      <button
                        onClick={() => setDeleteTarget({ type: 'bucket', key: b.name })}
                        className="text-xs text-red-500 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                        title="Delete bucket"
                      >
                        ✕
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ── Object Browser ─────────────────────────────────── */}
        {activeConnection && activeBucket && (
          <div className="space-y-4" onDragOver={handleDragOver} onDragLeave={handleDragLeave} onDrop={handleDrop}>
            {/* Breadcrumbs + actions bar */}
            <div className="card card-body">
              <div className="flex items-center justify-between flex-wrap gap-3">
                {/* Breadcrumbs */}
                <div className="flex items-center gap-1 text-sm min-w-0">
                  <button
                    onClick={() => navigateTo({ connection: activeConnection })}
                    className="text-[var(--accent)] hover:underline shrink-0"
                  >
                    Buckets
                  </button>
                  <span className="text-[var(--text-tertiary)] shrink-0">/</span>
                  <button
                    onClick={() => navigateTo({ connection: activeConnection, bucket: activeBucket })}
                    className="text-[var(--accent)] hover:underline truncate"
                  >
                    {activeBucket}
                  </button>
                  {breadcrumbParts().map(part => (
                    <span key={part.prefix} className="flex items-center gap-1 shrink-0">
                      <span className="text-[var(--text-tertiary)]">/</span>
                      <button
                        onClick={() => navigateTo({ connection: activeConnection, bucket: activeBucket, prefix: part.prefix })}
                        className="text-[var(--accent)] hover:underline truncate"
                      >
                        {part.label}
                      </button>
                    </span>
                  ))}
                </div>

                {/* Actions */}
                <div className="flex items-center gap-2 shrink-0">
                  {selectedObjects.size > 0 && (
                    <button
                      onClick={handleBatchDelete}
                      disabled={deleting}
                      className="btn text-sm text-red-500 border-red-500/30 hover:bg-red-500/10"
                    >
                      Delete ({selectedObjects.size})
                    </button>
                  )}
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={e => handleUpload(e.target.files, false)}
                  />
                  <input
                    ref={folderInputRef}
                    type="file"
                    multiple
                    {...{ webkitdirectory: 'true', webkitRelativePath: '' } as React.InputHTMLAttributes<HTMLInputElement>}
                    className="hidden"
                    onChange={e => handleUpload(e.target.files, true)}
                  />
                  <button
                    onClick={() => folderInputRef.current?.click()}
                    disabled={uploading}
                    className="btn btn-secondary text-sm"
                  >
                    {uploading ? 'Uploading...' : '↑ Folder'}
                  </button>
                  <button
                    onClick={() => fileInputRef.current?.click()}
                    disabled={uploading}
                    className="btn btn-primary text-sm"
                  >
                    {uploading ? `Uploading ${uploadProgress ? `${uploadProgress.done}/${uploadProgress.total}` : '...'}` : '↑ Upload'}
                  </button>
                </div>
              </div>
            </div>

            {/* Credential status indicator */}
            {credStatus && credStatus.source === 'admin' && (
              <div className="bg-amber-500/10 border border-amber-500/20 rounded-xl px-4 py-2 flex items-center gap-2">
                <svg className="w-4 h-4 text-amber-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                </svg>
                <p className="text-xs text-amber-600">
                  Using admin credentials. <Link href="/credentials" className="underline hover:no-underline">Add your own S3 credentials</Link> for personalized access.
                </p>
              </div>
            )}

            {/* Drag overlay */}
            {isDragging && (
              <div className="card border-2 border-dashed border-[var(--accent)] bg-[var(--accent)]/5 flex items-center justify-center py-8">
                <p className="text-[var(--accent)] font-medium">Drop files to upload</p>
              </div>
            )}

            {/* Object table */}
            {loadingObjects ? (
              <div className="card card-body text-center py-12">
                <p className="text-[13px] text-[var(--text-tertiary)]">Loading objects...</p>
              </div>
            ) : objects.length === 0 && folders.length === 0 ? (
              <div className="card text-center py-12" style={{ borderRadius: '12px' }}>
                <p className="text-[var(--text-secondary)] mb-2">This location is empty</p>
                <p className="text-xs text-[var(--text-tertiary)] mb-4">Upload files or drag & drop a folder</p>
                <div className="flex items-center justify-center gap-2">
                  <button onClick={() => folderInputRef.current?.click()} className="btn btn-secondary text-sm">
                    ↑ Upload Folder
                  </button>
                  <button onClick={() => fileInputRef.current?.click()} className="btn btn-primary text-sm">
                    ↑ Upload Files
                  </button>
                </div>
              </div>
            ) : (
              <div className="card overflow-hidden" style={{ borderRadius: '12px' }}>
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-[var(--border-light)]">
                      <th className="w-8 px-3 py-2.5 text-left">
                        <input
                          type="checkbox"
                          checked={objects.length > 0 && selectedObjects.size === objects.length}
                          onChange={e => {
                            if (e.target.checked) {
                              setSelectedObjects(new Set(objects.map(o => o.key)));
                            } else {
                              setSelectedObjects(new Set());
                            }
                          }}
                          className="w-3.5 h-3.5 rounded border-[var(--border)] text-[var(--accent)] focus:ring-[var(--accent)]"
                        />
                      </th>
                      <th className="px-3 py-2.5 text-left text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Name</th>
                      <th className="px-3 py-2.5 text-left text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Size</th>
                      <th className="px-3 py-2.5 text-left text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Type</th>
                      <th className="px-3 py-2.5 text-left text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Modified</th>
                      <th className="px-3 py-2.5 text-right text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wider">Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {/* Folders */}
                    {folders.map(f => {
                      const folderName = f.name.replace(currentPrefix, '').replace(/\/$/, '');
                      return (
                        <tr key={f.name} className="border-b border-[var(--border-light)] hover:bg-[var(--bg)] cursor-pointer"
                          onClick={() => navigateTo({ connection: activeConnection, bucket: activeBucket, prefix: f.name })}
                        >
                          <td className="px-3 py-2.5"></td>
                          <td className="px-3 py-2.5">
                            <div className="flex items-center gap-2">
                              <svg className="w-4 h-4 text-amber-500/70 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
                              </svg>
                              <span className="font-medium text-[var(--text-primary)]">{folderName}/</span>
                            </div>
                          </td>
                          <td className="px-3 py-2.5 text-[var(--text-tertiary)]">—</td>
                          <td className="px-3 py-2.5 text-[var(--text-tertiary)]">Folder</td>
                          <td className="px-3 py-2.5 text-[var(--text-tertiary)]">—</td>
                          <td className="px-3 py-2.5"></td>
                        </tr>
                      );
                    })}
                    {/* Objects */}
                    {objects.filter(o => !o.key.endsWith('/')).map(obj => {
                      const displayName = obj.key.replace(currentPrefix, '');
                      return (
                        <tr key={obj.key} className="border-b border-[var(--border-light)] hover:bg-[var(--bg)] group">
                          <td className="px-3 py-2.5">
                            <input
                              type="checkbox"
                              checked={selectedObjects.has(obj.key)}
                              onChange={() => toggleSelect(obj.key)}
                              className="w-3.5 h-3.5 rounded border-[var(--border)] text-[var(--accent)] focus:ring-[var(--accent)]"
                            />
                          </td>
                          <td className="px-3 py-2.5">
                            <div className="flex items-center gap-2">
                              <span className="w-5 h-5 rounded bg-[var(--border-light)] flex items-center justify-center text-[8px] font-bold text-[var(--text-tertiary)] shrink-0">
                                {fileIcon(obj.key)}
                              </span>
                              <span className="font-medium text-[var(--text-primary)] truncate" title={displayName}>
                                {displayName}
                              </span>
                            </div>
                          </td>
                          <td className="px-3 py-2.5 text-[var(--text-tertiary)]">{formatSize(obj.size)}</td>
                          <td className="px-3 py-2.5 text-[var(--text-tertiary)] max-w-[120px] truncate" title={obj.content_type}>
                            {obj.content_type || '—'}
                          </td>
                          <td className="px-3 py-2.5 text-[var(--text-tertiary)]">{formatDate(obj.last_modified)}</td>
                          <td className="px-3 py-2.5 text-right">
                            <div className="flex items-center justify-end gap-3 opacity-0 group-hover:opacity-100 transition-opacity">
                              {previewType(obj.key) && (
                                <button
                                  onClick={() => handlePreview(obj.key)}
                                  className="text-[var(--accent)] hover:text-[var(--accent)]/80"
                                  title="Preview"
                                >
                                  <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                    <path strokeLinecap="round" strokeLinejoin="round" d="M2.036 12.322a1.012 1.012 0 010-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178z" />
                                    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                                  </svg>
                                </button>
                              )}
                              <a
                                href={s3Browser.getDownloadUrl(activeConnection, activeBucket, obj.key)}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-[var(--accent)] hover:text-[var(--accent)]/80"
                                title="Download"
                              >
                                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                  <path strokeLinecap="round" strokeLinejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5M16.5 12L12 16.5m0 0L7.5 12m4.5 4.5V3" />
                                </svg>
                              </a>
                              <button
                                onClick={() => setDeleteTarget({ type: 'object', key: obj.key, bucket: activeBucket })}
                                className="text-red-500 hover:text-red-400"
                                title="Delete"
                              >
                                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                  <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
                                </svg>
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
          </div>
        )}

        {/* ── Create Bucket Modal ────────────────────────────── */}
        {showCreateBucket && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={() => setShowCreateBucket(false)}>
            <div className="bg-[var(--surface)] rounded-2xl p-6 max-w-sm w-full mx-4" onClick={e => e.stopPropagation()}>
              <h2 className="text-lg font-bold mb-4">Create Bucket</h2>
              <input
                type="text"
                value={newBucketName}
                onChange={e => setNewBucketName(e.target.value)}
                placeholder="my-bucket-name"
                className="w-full px-3 py-2 border border-[var(--border)] rounded-lg focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent mb-4"
                autoFocus
              />
              <p className="text-xs text-[var(--text-tertiary)] mb-4">
                Bucket names must be unique and DNS-compliant (lowercase, no spaces).
              </p>
              <div className="flex gap-3">
                <button
                  onClick={handleCreateBucket}
                  disabled={!newBucketName || creatingBucket}
                  className="flex-1 py-2 rounded-lg bg-[var(--accent)] text-white hover:opacity-90 disabled:opacity-50 transition-colors"
                >
                  {creatingBucket ? 'Creating...' : 'Create'}
                </button>
                <button
                  onClick={() => { setShowCreateBucket(false); setNewBucketName(''); }}
                  className="px-6 py-2 border border-[var(--border)] rounded-lg hover:bg-[var(--border-light)] transition-colors"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        )}

        {/* ── Preview Modal ──────────────────────────────────── */}
        {previewKey && (
          <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={closePreview}>
            <div
              className="bg-[var(--surface)] rounded-2xl w-full max-w-4xl max-h-[85vh] flex flex-col mx-4 overflow-hidden"
              onClick={e => e.stopPropagation()}
            >
              {/* Header */}
              <div className="flex items-center justify-between px-5 py-3 border-b border-[var(--border-light)] shrink-0">
                <div className="flex items-center gap-2 min-w-0">
                  <svg className="w-4 h-4 text-[var(--accent)] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M2.036 12.322a1.012 1.012 0 010-.639C3.423 7.51 7.36 4.5 12 4.5c4.638 0 8.573 3.007 9.963 7.178.07.207.07.431 0 .639C20.577 16.49 16.64 19.5 12 19.5c-4.638 0-8.573-3.007-9.963-7.178z" />
                    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  </svg>
                  <span className="font-medium text-sm text-[var(--text-primary)] truncate" title={previewKey}>
                    {previewKey.split('/').pop()}
                  </span>
                  <span className="text-xs text-[var(--text-tertiary)] shrink-0">{previewContentType}</span>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <a
                    href={s3Browser.getDownloadUrl(activeConnection, activeBucket, previewKey)}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-xs text-[var(--accent)] hover:underline"
                  >
                    ↓ Download
                  </a>
                  <button onClick={closePreview} className="text-[var(--text-tertiary)] hover:text-[var(--text-primary)]">
                    ✕
                  </button>
                </div>
              </div>

              {/* Body */}
              <div className="flex-1 overflow-auto p-5 min-h-0">
                {previewLoading ? (
                  <div className="flex items-center justify-center py-16">
                    <svg className="animate-spin w-5 h-5 text-[var(--accent)]" viewBox="0 0 24 24" fill="none">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    <span className="ml-2 text-sm text-[var(--text-tertiary)]">Loading preview...</span>
                  </div>
                ) : previewError ? (
                  <div className="text-center py-16">
                    <p className="text-sm text-red-500 mb-2">Preview failed</p>
                    <p className="text-xs text-[var(--text-tertiary)]">{previewError}</p>
                  </div>
                ) : previewType(previewKey) === 'image' && previewBlobUrl ? (
                  <div className="flex items-center justify-center">
                    <img
                      src={previewBlobUrl}
                      alt={previewKey.split('/').pop()}
                      className="max-w-full max-h-[65vh] rounded-lg"
                      style={{ imageRendering: 'auto' }}
                    />
                  </div>
                ) : previewType(previewKey) === 'pdf' && previewBlobUrl ? (
                  <iframe
                    src={previewBlobUrl}
                    className="w-full h-[65vh] rounded-lg border border-[var(--border-light)]"
                    title={previewKey.split('/').pop()}
                  />
                ) : (
                  <pre className="text-xs leading-relaxed text-[var(--text-secondary)] bg-[var(--bg)] rounded-lg p-4 overflow-auto whitespace-pre-wrap break-words font-mono">
                    {previewContent}
                  </pre>
                )}
              </div>
            </div>
          </div>
        )}

        {/* ── Delete Confirmation ────────────────────────────── */}
        <ConfirmModal
          open={!!deleteTarget}
          title={deleteTarget?.type === 'bucket' ? 'Delete Bucket' : 'Delete Object'}
          description={
            deleteTarget?.type === 'bucket'
              ? `Are you sure you want to delete bucket "${deleteTarget?.key}"? The bucket must be empty.`
              : `Are you sure you want to delete "${deleteTarget?.key}"?`
          }
          confirmLabel="Delete"
          variant="danger"
          loading={deleting}
          onConfirm={handleDelete}
          onCancel={() => setDeleteTarget(null)}
        />
      </div>
    </div>
  );
}

