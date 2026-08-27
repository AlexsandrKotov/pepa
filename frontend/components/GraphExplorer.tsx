'use client';

import { useCallback, useState, useEffect } from 'react';
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
  MarkerType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

interface GraphNode {
  id: string;
  name: string;
  type_key: string;
  status: string;
  metadata?: Record<string, unknown>;
}

interface GraphEdge {
  id: string;
  source_id: string;
  target_id: string;
  type_key: string;
}

interface GraphExplorerProps {
  initialNodes: GraphNode[];
  initialEdges: GraphEdge[];
}

export default function GraphExplorer({ initialNodes, initialEdges }: GraphExplorerProps) {
  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesState] = useEdgesState<Edge>([]);
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);

  useEffect(() => {
    // Convert API data to React Flow format
    const flowNodes: Node[] = initialNodes.map((node, index) => ({
      id: node.id,
      position: { x: (index % 5) * 200, y: Math.floor(index / 5) * 150 },
      data: {
        label: (
          <div className="px-3 py-2">
            <div className="text-[13px] font-medium text-[#171717]">{node.name}</div>
            <div className="flex items-center gap-1.5 mt-1">
              <span className="text-[11px] text-[#a3a3a3]">{node.type_key}</span>
              <span className={`w-1.5 h-1.5 rounded-full ${
                node.status === 'active' ? 'bg-emerald-500' :
                node.status === 'deprecated' ? 'bg-red-500' :
                'bg-amber-500'
              }`} />
            </div>
          </div>
        ),
        node,
      },
      style: {
        background: '#ffffff',
        border: '1px solid #e5e5e5',
        borderRadius: '6px',
        width: 180,
      },
    }));

    const flowEdges: Edge[] = initialEdges.map((edge) => ({
      id: edge.id,
      source: edge.source_id,
      target: edge.target_id,
      label: edge.type_key,
      labelStyle: { fontSize: 11, fill: '#a3a3a3' },
      labelBgStyle: { fill: '#fafafa', fillOpacity: 0.9 },
      labelBgPadding: [4, 2] as [number, number],
      labelBgBorderRadius: 3,
      style: { stroke: '#d4d4d4', strokeWidth: 1 },
      markerEnd: {
        type: MarkerType.ArrowClosed,
        color: '#d4d4d4',
      },
    }));

    setNodes(flowNodes);
    setEdges(flowEdges);
  }, [initialNodes, initialEdges, setNodes, setEdges]);

  const onNodeClick = useCallback((_: unknown, node: Node) => {
    setSelectedNode(node.data.node as GraphNode);
  }, []);

  const onEdgesChange = useCallback(() => {}, []);

  return (
    <div className="flex h-[600px] border border-[#e5e5e5] rounded-lg overflow-hidden">
      <div className="flex-1">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={onNodeClick}
          fitView
          attributionPosition="bottom-left"
        >
          <Controls />
          <Background color="#f5f5f5" gap={20} />
          <MiniMap
            nodeColor={(node) => {
              const status = (node.data?.node as GraphNode)?.status;
              return status === 'active' ? '#dcfce7' :
                     status === 'deprecated' ? '#fee2e2' :
                     '#fef9c3';
            }}
            maskColor="rgba(0,0,0,0.05)"
          />
        </ReactFlow>
      </div>
      
      {selectedNode && (
        <div className="w-[280px] border-l border-[#e5e5e5] bg-white p-4 overflow-y-auto">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-[14px] font-medium text-[#171717]">Node Details</h3>
            <button
              onClick={() => setSelectedNode(null)}
              className="text-[#a3a3a3] hover:text-[#171717] transition-colors"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
          
          <div className="space-y-3">
            <div>
              <label className="text-[11px] text-[#a3a3a3] uppercase tracking-wide">Name</label>
              <p className="text-[13px] text-[#171717] font-medium">{selectedNode.name}</p>
            </div>
            <div>
              <label className="text-[11px] text-[#a3a3a3] uppercase tracking-wide">Type</label>
              <p className="text-[13px] text-[#171717]">{selectedNode.type_key}</p>
            </div>
            <div>
              <label className="text-[11px] text-[#a3a3a3] uppercase tracking-wide">Status</label>
              <p className="text-[13px] text-[#171717]">{selectedNode.status}</p>
            </div>
            {selectedNode.metadata && Object.keys(selectedNode.metadata).length > 0 && (
              <div>
                <label className="text-[11px] text-[#a3a3a3] uppercase tracking-wide">Metadata</label>
                <pre className="text-[11px] text-[#525252] bg-[#fafafa] rounded p-2 mt-1 overflow-x-auto">
                  {JSON.stringify(selectedNode.metadata, null, 2)}
                </pre>
              </div>
            )}
            <a
              href={`/entities?id=${selectedNode.id}`}
              className="btn btn-secondary btn-sm w-full justify-center mt-4"
            >
              View Entity
            </a>
          </div>
        </div>
      )}
    </div>
  );
}
