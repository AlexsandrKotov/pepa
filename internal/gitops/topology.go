package gitops

import (
	"context"
	"fmt"
	"sort"
)

// TopologyNode represents a node in the dependency graph.
type TopologyNode struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"` // HelmRelease, Kustomization, Application, HelmRepository, GitRepository
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Health     string   `json:"health"`      // healthy, progressing, degraded, unknown
	SyncStatus string   `json:"sync_status"` // synced, out_of_sync, unknown
	Children   []string `json:"children"`    // node IDs this depends on
	Parents    []string `json:"parents"`     // node IDs that depend on this
	FilePath   string   `json:"file_path,omitempty"`
	// Layout position (computed)
	PositionX float64 `json:"position_x"`
	PositionY float64 `json:"position_y"`
}

// TopologyEdge represents a directed dependency edge.
type TopologyEdge struct {
	From string `json:"from"` // dependent node
	To   string `json:"to"`   // dependency node
	Type string `json:"type"` // depends_on, source_ref, values_from
}

// TopologyGraph holds the full dependency graph.
type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// BuildTopology constructs a dependency graph from scanned resources.
func BuildTopology(ctx context.Context, repo *Repo, resources []Resource) *TopologyGraph {
	graph := &TopologyGraph{
		Nodes: make([]TopologyNode, 0),
		Edges: make([]TopologyEdge, 0),
	}

	nodeMap := make(map[string]*TopologyNode)

	// Create nodes for each resource
	for _, r := range resources {
		nodeID := resourceNodeID(r)
		health := "unknown"
		syncStatus := "unknown"
		// Use health from labels if present (set by live cluster queries)
		if h, ok := r.Labels["health"]; ok && h != "" {
			health = h
		}
		if s, ok := r.Labels["sync_status"]; ok && s != "" {
			syncStatus = s
		}
		node := &TopologyNode{
			ID:         nodeID,
			Kind:       r.Kind,
			Name:       r.Name,
			Namespace:  r.Namespace,
			Health:     health,
			SyncStatus: syncStatus,
			FilePath:   r.FilePath,
			Children:   make([]string, 0),
			Parents:    make([]string, 0),
		}
		nodeMap[nodeID] = node
		graph.Nodes = append(graph.Nodes, *node)
	}

	// Create implicit nodes for referenced resources (HelmRepository, GitRepository)
	for _, r := range resources {
		switch r.Kind {
		case "HelmRelease":
			if r.Repo != "" {
				refID := fmt.Sprintf("HelmRepository/%s/%s", r.Namespace, r.Repo)
				if _, exists := nodeMap[refID]; !exists {
					refNode := &TopologyNode{
						ID:         refID,
						Kind:       "HelmRepository",
						Name:       r.Repo,
						Namespace:  r.Namespace,
						Health:     "unknown",
						SyncStatus: "unknown",
						Children:   make([]string, 0),
						Parents:    make([]string, 0),
					}
					nodeMap[refID] = refNode
					graph.Nodes = append(graph.Nodes, *refNode)
				}
				// Edge: HelmRelease -> HelmRepository
				hrID := resourceNodeID(r)
				graph.Edges = append(graph.Edges, TopologyEdge{
					From: hrID,
					To:   refID,
					Type: "source_ref",
				})
				nodeMap[hrID].Children = append(nodeMap[hrID].Children, refID)
				nodeMap[refID].Parents = append(nodeMap[refID].Parents, hrID)
			}

			// valuesFrom references
			for _, vf := range r.ValuesFrom {
				vfID := fmt.Sprintf("%s/%s/%s", vf.Kind, r.Namespace, vf.Name)
				if _, exists := nodeMap[vfID]; !exists {
					vfNode := &TopologyNode{
						ID:         vfID,
						Kind:       vf.Kind,
						Name:       vf.Name,
						Namespace:  r.Namespace,
						Health:     "unknown",
						SyncStatus: "unknown",
						Children:   make([]string, 0),
						Parents:    make([]string, 0),
					}
					nodeMap[vfID] = vfNode
					graph.Nodes = append(graph.Nodes, *vfNode)
				}
				hrID := resourceNodeID(r)
				graph.Edges = append(graph.Edges, TopologyEdge{
					From: hrID,
					To:   vfID,
					Type: "values_from",
				})
				nodeMap[hrID].Children = append(nodeMap[hrID].Children, vfID)
				nodeMap[vfID].Parents = append(nodeMap[vfID].Parents, hrID)
			}

		case "Kustomization":
			// Kustomization -> GitRepository (implicit source)
			ksID := resourceNodeID(r)
			gitRefID := fmt.Sprintf("GitRepository/%s/%s", r.Namespace, r.Name)
			if _, exists := nodeMap[gitRefID]; !exists {
				gitNode := &TopologyNode{
					ID:         gitRefID,
					Kind:       "GitRepository",
					Name:       r.Name,
					Namespace:  r.Namespace,
					Health:     "unknown",
					SyncStatus: "unknown",
					Children:   make([]string, 0),
					Parents:    make([]string, 0),
				}
				nodeMap[gitRefID] = gitNode
				graph.Nodes = append(graph.Nodes, *gitNode)
			}
			graph.Edges = append(graph.Edges, TopologyEdge{
				From: ksID,
				To:   gitRefID,
				Type: "source_ref",
			})
			nodeMap[ksID].Children = append(nodeMap[ksID].Children, gitRefID)
			nodeMap[gitRefID].Parents = append(nodeMap[gitRefID].Parents, ksID)
		}

		// DependsOn edges (FluxCD)
		for _, dep := range r.DependsOn {
			depID := findNodeID(nodeMap, dep, r.Namespace)
			if depID == "" {
				// Create implicit dependency node
				depID = fmt.Sprintf("Unknown/%s/%s", r.Namespace, dep)
				if _, exists := nodeMap[depID]; !exists {
					depNode := &TopologyNode{
						ID:         depID,
						Kind:       "Unknown",
						Name:       dep,
						Namespace:  r.Namespace,
						Health:     "unknown",
						SyncStatus: "unknown",
						Children:   make([]string, 0),
						Parents:    make([]string, 0),
					}
					nodeMap[depID] = depNode
					graph.Nodes = append(graph.Nodes, *depNode)
				}
			}
			myID := resourceNodeID(r)
			graph.Edges = append(graph.Edges, TopologyEdge{
				From: myID,
				To:   depID,
				Type: "depends_on",
			})
			nodeMap[myID].Children = append(nodeMap[myID].Children, depID)
			nodeMap[depID].Parents = append(nodeMap[depID].Parents, myID)
		}

		// ArgoCD source reference
		if r.Source != nil && r.Source.RepoURL != "" {
			myID := resourceNodeID(r)
			srcID := fmt.Sprintf("ArgoSource/%s", sanitizeID(r.Source.RepoURL))
			if _, exists := nodeMap[srcID]; !exists {
				srcNode := &TopologyNode{
					ID:         srcID,
					Kind:       "ArgoSource",
					Name:       r.Source.RepoURL,
					Namespace:  r.Namespace,
					Health:     "unknown",
					SyncStatus: "unknown",
					Children:   make([]string, 0),
					Parents:    make([]string, 0),
				}
				nodeMap[srcID] = srcNode
				graph.Nodes = append(graph.Nodes, *srcNode)
			}
			graph.Edges = append(graph.Edges, TopologyEdge{
				From: myID,
				To:   srcID,
				Type: "source_ref",
			})
			nodeMap[myID].Children = append(nodeMap[myID].Children, srcID)
			nodeMap[srcID].Parents = append(nodeMap[srcID].Parents, myID)
		}
	}

	// Deduplicate children/parents
	for _, node := range graph.Nodes {
		if n, ok := nodeMap[node.ID]; ok {
			n.Children = dedup(n.Children)
			n.Parents = dedup(n.Parents)
		}
	}

	// Compute layout (layered DAG layout)
	computeLayout(graph)

	// Copy back updated children/parents from nodeMap, preserving layout positions
	for i, n := range graph.Nodes {
		if updated, ok := nodeMap[n.ID]; ok {
			px := graph.Nodes[i].PositionX
			py := graph.Nodes[i].PositionY
			graph.Nodes[i] = *updated
			graph.Nodes[i].PositionX = px
			graph.Nodes[i].PositionY = py
		}
	}

	return graph
}

// resourceNodeID creates a unique ID for a resource node.
func resourceNodeID(r Resource) string {
	return fmt.Sprintf("%s/%s/%s", r.Kind, r.Namespace, r.Name)
}

// findNodeID looks for a node by name (optionally namespace-qualified).
func findNodeID(nodeMap map[string]*TopologyNode, name, defaultNS string) string {
	// Try exact match with namespace
	candidates := []string{
		fmt.Sprintf("Kustomization/%s/%s", defaultNS, name),
		fmt.Sprintf("HelmRelease/%s/%s", defaultNS, name),
		fmt.Sprintf("Application/%s/%s", defaultNS, name),
		fmt.Sprintf("Kustomization//%s", name),
		fmt.Sprintf("HelmRelease//%s", name),
		fmt.Sprintf("Application//%s", name),
	}
	for _, c := range candidates {
		if _, ok := nodeMap[c]; ok {
			return c
		}
	}
	return ""
}

// computeLayout assigns x,y positions using a layered DAG layout with centering.
func computeLayout(graph *TopologyGraph) {
	if len(graph.Nodes) == 0 {
		return
	}

	// Build adjacency for topological sort
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for _, n := range graph.Nodes {
		inDegree[n.ID] = 0
	}
	for _, e := range graph.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
	}

	// Topological sort (Kahn's algorithm) to assign layers
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	layers := make(map[string]int)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		layer := layers[id]
		for _, neighbor := range adj[id] {
			if l, ok := layers[neighbor]; !ok || l < layer+1 {
				layers[neighbor] = layer + 1
			}
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Assign any un-layered nodes (cycles or disconnected)
	maxLayer := 0
	for _, l := range layers {
		if l > maxLayer {
			maxLayer = l
		}
	}
	for _, n := range graph.Nodes {
		if _, ok := layers[n.ID]; !ok {
			layers[n.ID] = maxLayer + 1
		}
	}

	// Group by layer
	layerGroups := make(map[int][]string)
	for id, layer := range layers {
		layerGroups[layer] = append(layerGroups[layer], id)
	}

	// Sort within each layer by kind then name for visual grouping
	kindOrder := map[string]int{
		"GitRepository":  0,
		"HelmRepository": 1,
		"ArgoSource":     1,
		"Kustomization":  2,
		"HelmRelease":    3,
		"Application":    3,
	}
	for _, group := range layerGroups {
		sort.Slice(group, func(i, j int) bool {
			ni := findNode(graph.Nodes, group[i])
			nj := findNode(graph.Nodes, group[j])
			oi := kindOrder[ni.Kind]
			oj := kindOrder[nj.Kind]
			if oi != oj {
				return oi < oj
			}
			return ni.Name < nj.Name
		})
	}

	// Find max layer size for vertical centering
	maxGroupSize := 0
	for _, group := range layerGroups {
		if len(group) > maxGroupSize {
			maxGroupSize = len(group)
		}
	}

	// Assign positions with generous spacing for ArgoCD-style cards
	const xSpacing = 320.0
	const ySpacing = 110.0

	for layer, ids := range layerGroups {
		groupSize := len(ids)
		// Center this layer vertically relative to the tallest layer
		yOffset := float64(maxGroupSize-groupSize) * ySpacing / 2.0
		for i, id := range ids {
			for j, n := range graph.Nodes {
				if n.ID == id {
					graph.Nodes[j].PositionX = float64(layer) * xSpacing
					graph.Nodes[j].PositionY = yOffset + float64(i)*ySpacing
					break
				}
			}
		}
	}
}

// findNode returns the node with the given ID.
func findNode(nodes []TopologyNode, id string) TopologyNode {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
	}
	return TopologyNode{}
}

func sanitizeID(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			if c < 128 {
				result = append(result, byte(c)) //nolint:gosec // G115: c < 128 guarantees safe byte range
			}
		} else {
			result = append(result, '_')
		}
	}
	if len(result) > 60 {
		result = result[:60]
	}
	return string(result)
}

func dedup(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
