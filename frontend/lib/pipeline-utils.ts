// Pipeline builder utilities - DAG validation, auto-layout, spec conversion

export interface PipelineNode {
  id: string;
  name: string;
  type: string;
  plugin?: string;
  action?: string;
  params?: Record<string, unknown>;
  dependsOn?: string[];
  runWhen?: string;
  position: { x: number; y: number };
  status?: 'idle' | 'success' | 'failed';
}

export interface PipelineEdge {
  id: string;
  source: string;
  target: string;
}

// Convert step spec to pipeline node
export function stepToNode(step: any, index: number): PipelineNode {
  return {
    id: step.name || `step-${index}`,
    name: step.name || `step-${index}`,
    type: step.type || 'task',
    plugin: step.plugin,
    action: step.action,
    params: step.params || {},
    dependsOn: step.depends_on || [],
    runWhen: step.run_when,
    position: { x: 0, y: 0 }, // Will be set by auto-layout
  };
}

// Convert pipeline node to step spec
export function nodeToStep(node: PipelineNode): any {
  return {
    name: node.name,
    type: node.type,
    plugin: node.plugin,
    action: node.action,
    params: node.params || {},
    depends_on: node.dependsOn || [],
    run_when: node.runWhen,
  };
}

// Convert edges to depends_on relationships
export function edgesToDependsOn(edges: PipelineEdge[], nodeId: string): string[] {
  return edges
    .filter(e => e.target === nodeId)
    .map(e => e.source);
}

// Auto-layout algorithm (simple DAG layering)
export function autoLayout(nodes: PipelineNode[], edges: PipelineEdge[]): PipelineNode[] {
  // Build adjacency list
  const adj = new Map<string, string[]>();
  const inDegree = new Map<string, number>();
  
  nodes.forEach(n => {
    adj.set(n.id, []);
    inDegree.set(n.id, 0);
  });
  
  edges.forEach(e => {
    adj.get(e.source)?.push(e.target);
    inDegree.set(e.target, (inDegree.get(e.target) || 0) + 1);
  });
  
  // Topological sort with layers
  const layers: string[][] = [];
  const queue: string[] = [];
  
  // Start with nodes that have no dependencies
  inDegree.forEach((deg, id) => {
    if (deg === 0) queue.push(id);
  });
  
  while (queue.length > 0) {
    const layer = [...queue];
    layers.push(layer);
    queue.length = 0;
    
    layer.forEach(nodeId => {
      adj.get(nodeId)?.forEach(target => {
        const newDeg = (inDegree.get(target) || 0) - 1;
        inDegree.set(target, newDeg);
        if (newDeg === 0) queue.push(target);
      });
    });
  }
  
  // Position nodes by layer
  const nodeWidth = 250;
  const nodeHeight = 150;
  const horizontalGap = 100;
  const verticalGap = 80;
  
  const positioned = nodes.map(n => ({ ...n }));
  
  layers.forEach((layer, layerIndex) => {
    layer.forEach((nodeId, nodeIndex) => {
      const node = positioned.find(n => n.id === nodeId);
      if (node) {
        node.position = {
          x: layerIndex * (nodeWidth + horizontalGap),
          y: nodeIndex * (nodeHeight + verticalGap),
        };
      }
    });
  });
  
  return positioned;
}

// Validate DAG for cycles
export function validateDAG(nodes: PipelineNode[], edges: PipelineEdge[]): string[] {
  const errors: string[] = [];
  
  // Check for duplicate names
  const names = new Set<string>();
  nodes.forEach(n => {
    if (names.has(n.name)) {
      errors.push(`Duplicate step name: ${n.name}`);
    }
    names.add(n.name);
  });
  
  // Check for cycles using DFS
  const adj = new Map<string, string[]>();
  nodes.forEach(n => adj.set(n.id, []));
  edges.forEach(e => {
    adj.get(e.source)?.push(e.target);
  });
  
  const visited = new Set<string>();
  const recStack = new Set<string>();
  
  function hasCycle(nodeId: string): boolean {
    if (!visited.has(nodeId)) {
      visited.add(nodeId);
      recStack.add(nodeId);
      
      const neighbors = adj.get(nodeId) || [];
      for (const neighbor of neighbors) {
        if (!visited.has(neighbor) && hasCycle(neighbor)) {
          return true;
        } else if (recStack.has(neighbor)) {
          return true;
        }
      }
    }
    recStack.delete(nodeId);
    return false;
  }
  
  nodes.forEach(n => {
    if (hasCycle(n.id)) {
      errors.push('Circular dependency detected in pipeline');
    }
  });
  
  return errors;
}

// Generate unique step name
export function generateStepName(existing: string[], base: string = 'step'): string {
  let counter = 1;
  let name = `${base}-${counter}`;
  while (existing.includes(name)) {
    counter++;
    name = `${base}-${counter}`;
  }
  return name;
}
