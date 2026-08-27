'use client';

import { useState } from 'react';
import { type GitopsFileNode, type GitopsResource } from '@/lib/api';

interface ResourceTreeProps {
  tree: GitopsFileNode;
  onResourceSelect?: (resource: GitopsResource) => void;
  selectedResource?: GitopsResource | null;
}

interface TreeNodeProps {
  node: GitopsFileNode;
  level: number;
  onResourceSelect?: (resource: GitopsResource) => void;
  selectedResource?: GitopsResource | null;
}

function nodeIcon(type: string) {
  switch (type) {
    case 'environment': return '🌍';
    case 'cluster': return '🏢';
    case 'subcluster': return '📍';
    case 'dir': return '📁';
    default: return '📄';
  }
}

function kindIcon(kind: string) {
  switch (kind) {
    case 'HelmRelease': return '📦';
    case 'Kustomization': return '🔧';
    case 'Application': return '🚀';
    case 'Kustomize': return '📋';
    default: return '📄';
  }
}

function TreeNode({ node, level, onResourceSelect, selectedResource }: TreeNodeProps) {
  const [expanded, setExpanded] = useState(level < 2); // Auto-expand first 2 levels

  const hasChildren = node.children && node.children.length > 0;
  const hasResources = node.resources && node.resources.length > 0;
  const isExpandable = hasChildren || hasResources;

  const handleClick = () => {
    if (isExpandable) {
      setExpanded(!expanded);
    }
  };

  return (
    <div>
      <div
        onClick={handleClick}
        className={`flex items-center gap-2 px-3 py-2 hover:bg-[var(--bg)] transition-colors cursor-pointer border-b border-[var(--border-light)] ${
          isExpandable ? '' : 'cursor-default'
        }`}
        style={{ paddingLeft: `${level * 16 + 12}px` }}
      >
        {/* Expand/Collapse Icon */}
        {isExpandable ? (
          <span className={`text-[10px] text-[var(--text-tertiary)] transition-transform ${expanded ? 'rotate-90' : ''}`}>
            ▶
          </span>
        ) : (
          <span className="text-[10px] w-2.5" />
        )}

        {/* Node Icon */}
        <span className="text-sm">{nodeIcon(node.type)}</span>

        {/* Node Name */}
        <span className={`text-[12px] flex-1 ${
          node.type === 'environment' ? 'font-semibold text-[var(--text-primary)]' :
          node.type === 'subcluster' ? 'font-medium text-[var(--text-secondary)]' :
          'text-[var(--text-secondary)]'
        }`}>
          {node.name}
        </span>

        {/* Resource Count Badge */}
        {node.count && node.count > 0 && (
          <span className="text-[10px] px-2 py-0.5 rounded-full bg-[var(--bg)] text-[var(--text-tertiary)] font-medium">
            {node.count} {node.count === 1 ? 'resource' : 'resources'}
          </span>
        )}
      </div>

      {/* Children */}
      {expanded && hasChildren && (
        <div>
          {node.children!.map((child, idx) => (
            <TreeNode
              key={`${child.path}-${idx}`}
              node={child}
              level={level + 1}
              onResourceSelect={onResourceSelect}
              selectedResource={selectedResource}
            />
          ))}
        </div>
      )}

      {/* Resources */}
      {expanded && hasResources && (
        <div>
          {node.resources!.map((resource, idx) => (
            <div
              key={`${resource.name}-${idx}`}
              onClick={() => onResourceSelect?.(resource)}
              className={`flex items-center gap-2 px-3 py-2 hover:bg-[var(--accent-subtle)] transition-colors cursor-pointer border-b border-[var(--border-light)] ${
                selectedResource === resource ? 'bg-[var(--accent-subtle)]' : ''
              }`}
              style={{ paddingLeft: `${(level + 1) * 16 + 12}px` }}
            >
              <span className="text-[10px] w-2.5" />
              <span className="text-sm">{kindIcon(resource.kind)}</span>
              <div className="flex-1 min-w-0">
                <div className="text-[12px] text-[var(--text-primary)] truncate">
                  {resource.name}
                </div>
                {resource.namespace && (
                  <div className="text-[10px] text-[var(--text-tertiary)]">
                    ns: {resource.namespace}
                  </div>
                )}
              </div>
              {resource.chart && (
                <span className="text-[10px] font-mono text-[var(--text-tertiary)] truncate max-w-[120px]">
                  {resource.chart}{resource.version ? `@${resource.version}` : ''}
                </span>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export default function ResourceTree({ tree, onResourceSelect, selectedResource }: ResourceTreeProps) {
  if (!tree || (!tree.children?.length && !tree.resources?.length)) {
    return (
      <div className="p-8 text-center text-[13px] text-[var(--text-tertiary)]">
        No resources found in tree structure
      </div>
    );
  }

  return (
    <div className="border border-[var(--border)] rounded-lg overflow-hidden">
      {/* Tree Header */}
      <div className="px-4 py-2 bg-[var(--bg)] border-b border-[var(--border)]">
        <h3 className="text-[11px] font-semibold text-[var(--text-tertiary)] uppercase tracking-wider">
          Repository Structure
        </h3>
      </div>

      {/* Tree Content */}
      <div className="divide-y divide-[var(--border-light)]">
        {tree.children?.map((child, idx) => (
          <TreeNode
            key={`${child.path}-${idx}`}
            node={child}
            level={0}
            onResourceSelect={onResourceSelect}
            selectedResource={selectedResource}
          />
        ))}
        {tree.resources?.map((resource, idx) => (
          <div
            key={`${resource.name}-${idx}`}
            onClick={() => onResourceSelect?.(resource)}
            className={`flex items-center gap-2 px-3 py-2 hover:bg-[var(--accent-subtle)] transition-colors cursor-pointer border-b border-[var(--border-light)] ${
              selectedResource === resource ? 'bg-[var(--accent-subtle)]' : ''
            }`}
          >
            <span className="text-sm">{kindIcon(resource.kind)}</span>
            <div className="flex-1 min-w-0">
              <div className="text-[12px] text-[var(--text-primary)] truncate">
                {resource.name}
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
