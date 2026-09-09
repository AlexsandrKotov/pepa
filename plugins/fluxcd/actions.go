package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// FluxCD GVR (GroupVersionResource) definitions
var (
	kustomizationGVR = schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}
	helmReleaseGVR = schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}
	gitRepositoryGVR = schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "gitrepositories",
	}
	helmRepositoryGVR = schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "helmrepositories",
	}
)

// FluxController handles FluxCD CRD operations via dynamic Kubernetes client.
type FluxController struct {
	client     dynamic.Interface
	restConfig *rest.Config
}

// NewFluxController creates a new controller from kubeconfig bytes.
func NewFluxController(kubeconfig []byte) (*FluxController, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &FluxController{client: client, restConfig: config}, nil
}

// ── Kustomization Actions ─────────────────────────────────────

func (p *FluxCDPlugin) listKustomizations(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	// When no namespace is specified, list across all namespaces so the
	// engine client discovers every FluxCD Kustomization in the cluster.
	var list *unstructured.UnstructuredList
	var err error
	if input.Namespace != "" {
		list, err = fc.client.Resource(kustomizationGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = fc.client.Resource(kustomizationGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list kustomizations: %w", err)
	}

	type KustomizationSummary struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Ready     string `json:"ready"`
		Revision  string `json:"revision,omitempty"`
		Suspended bool   `json:"suspended"`
		Age       string `json:"age"`
	}

	items := make([]KustomizationSummary, 0, len(list.Items))
	for _, item := range list.Items {
		conditions, _ := getNestedSlice(item.Object, "status", "conditions")
		ready := "Unknown"
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					ready = fmt.Sprintf("%v", cm["status"])
					if msg, ok := cm["message"].(string); ok && ready == "False" {
						ready = "False: " + msg
					}
					break
				}
			}
		}

		revision, _ := getNestedString(item.Object, "status", "lastAppliedRevision")
		if revision == "" {
			revision, _ = getNestedString(item.Object, "status", "lastAttemptedRevision")
		}

		suspended := false
		if s, ok := item.Object["spec"].(map[string]interface{}); ok {
			if sv, ok := s["suspend"].(bool); ok {
				suspended = sv
			}
		}

		items = append(items, KustomizationSummary{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			Ready:     ready,
			Revision:  revision,
			Suspended: suspended,
			Age:       formatAge(item.GetCreationTimestamp().Time),
		})
	}

	return actionOutput(items)
}

func (p *FluxCDPlugin) getKustomization(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	obj, err := fc.client.Resource(kustomizationGVR).Namespace(input.Namespace).Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get kustomization: %w", err)
	}

	return actionOutput(flattenFluxResource(obj))
}

func (p *FluxCDPlugin) reconcileKustomization(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	err := fc.triggerReconcile(ctx, kustomizationGVR, input.Namespace, input.Name)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]string{
		"status":    "success",
		"message":   fmt.Sprintf("reconcile triggered for %s/%s", input.Namespace, input.Name),
		"resource":  "kustomization",
		"name":      input.Name,
		"namespace": input.Namespace,
	})
}

// ── HelmRelease Actions ───────────────────────────────────────

func (p *FluxCDPlugin) listHelmReleases(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	// When no namespace is specified, list across all namespaces so the
	// engine client discovers every FluxCD HelmRelease in the cluster.
	var list *unstructured.UnstructuredList
	var err error
	if input.Namespace != "" {
		list, err = fc.client.Resource(helmReleaseGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = fc.client.Resource(helmReleaseGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list helmreleases: %w", err)
	}

	type HelmReleaseSummary struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Ready     string `json:"ready"`
		Revision  string `json:"revision,omitempty"`
		Suspended bool   `json:"suspended"`
		Age       string `json:"age"`
	}

	items := make([]HelmReleaseSummary, 0, len(list.Items))
	for _, item := range list.Items {
		conditions, _ := getNestedSlice(item.Object, "status", "conditions")
		ready := "Unknown"
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					ready = fmt.Sprintf("%v", cm["status"])
					if msg, ok := cm["message"].(string); ok && ready == "False" {
						ready = "False: " + msg
					}
					break
				}
			}
		}

		revision, _ := getNestedString(item.Object, "status", "lastAppliedRevision")
		if revision == "" {
			revision, _ = getNestedString(item.Object, "status", "lastAttemptedRevision")
		}

		suspended := false
		if s, ok := item.Object["spec"].(map[string]interface{}); ok {
			if sv, ok := s["suspend"].(bool); ok {
				suspended = sv
			}
		}

		items = append(items, HelmReleaseSummary{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			Ready:     ready,
			Revision:  revision,
			Suspended: suspended,
			Age:       formatAge(item.GetCreationTimestamp().Time),
		})
	}

	return actionOutput(items)
}

func (p *FluxCDPlugin) getHelmRelease(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	obj, err := fc.client.Resource(helmReleaseGVR).Namespace(input.Namespace).Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get helmrelease: %w", err)
	}

	return actionOutput(flattenFluxResource(obj))
}

func (p *FluxCDPlugin) reconcileHelmRelease(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	err := fc.triggerReconcile(ctx, helmReleaseGVR, input.Namespace, input.Name)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]string{
		"status":    "success",
		"message":   fmt.Sprintf("reconcile triggered for %s/%s", input.Namespace, input.Name),
		"resource":  "helmrelease",
		"name":      input.Name,
		"namespace": input.Namespace,
	})
}

// ── Suspend / Resume Actions ──────────────────────────────────

func (p *FluxCDPlugin) suspend(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Resource  string `json:"resource"` // "kustomization" or "helmrelease"
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	gvr, err := p.resolveGVR(input.Resource)
	if err != nil {
		return nil, err
	}

	err = fc.setSuspend(ctx, gvr, input.Namespace, input.Name, true)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]string{
		"status":    "success",
		"message":   fmt.Sprintf("%s/%s suspended", input.Namespace, input.Name),
		"resource":  input.Resource,
		"name":      input.Name,
		"namespace": input.Namespace,
	})
}

func (p *FluxCDPlugin) resume(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Resource  string `json:"resource"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	gvr, err := p.resolveGVR(input.Resource)
	if err != nil {
		return nil, err
	}

	err = fc.setSuspend(ctx, gvr, input.Namespace, input.Name, false)
	if err != nil {
		return nil, err
	}

	return actionOutput(map[string]string{
		"status":    "success",
		"message":   fmt.Sprintf("%s/%s resumed", input.Namespace, input.Name),
		"resource":  input.Resource,
		"name":      input.Name,
		"namespace": input.Namespace,
	})
}

// ── Health Action ─────────────────────────────────────────────

func (p *FluxCDPlugin) getHealth(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	type ResourceHealth struct {
		Kind      string `json:"kind"`
		Total     int    `json:"total"`
		Ready     int    `json:"ready"`
		NotReady  int    `json:"not_ready"`
		Suspended int    `json:"suspended"`
	}

	health := struct {
		Kustomizations ResourceHealth `json:"kustomizations"`
		HelmReleases   ResourceHealth `json:"helmreleases"`
		Namespace      string         `json:"namespace"`
	}{
		Namespace: input.Namespace,
	}

	// Check Kustomizations
	kustList, err := fc.client.Resource(kustomizationGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		health.Kustomizations = analyzeFluxHealth(kustList.Items)
		health.Kustomizations.Kind = "Kustomization"
	}

	// Check HelmReleases
	hrList, err := fc.client.Resource(helmReleaseGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		health.HelmReleases = analyzeFluxHealth(hrList.Items)
		health.HelmReleases.Kind = "HelmRelease"
	}

	return actionOutput(health)
}

// ── FluxController Helpers ────────────────────────────────────

func (fc *FluxController) triggerReconcile(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) error {
	obj, err := fc.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get resource %s/%s: %w", namespace, name, err)
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["reconcile.fluxcd.io/requestedAt"] = time.Now().Format(time.RFC3339Nano)
	obj.SetAnnotations(annotations)

	_, err = fc.client.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update resource for reconcile: %w", err)
	}

	return nil
}

func (fc *FluxController) setSuspend(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string, suspend bool) error {
	obj, err := fc.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get resource %s/%s: %w", namespace, name, err)
	}

	spec, ok := obj.Object["spec"].(map[string]interface{})
	if !ok {
		spec = make(map[string]interface{})
		obj.Object["spec"] = spec
	}
	spec["suspend"] = suspend

	_, err = fc.client.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update resource suspend: %w", err)
	}

	return nil
}

// ── Plugin Helpers ────────────────────────────────────────────

func (p *FluxCDPlugin) resolveGVR(resource string) (schema.GroupVersionResource, error) {
	switch resource {
	case "kustomization", "kustomizations":
		return kustomizationGVR, nil
	case "helmrelease", "helmreleases":
		return helmReleaseGVR, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unknown resource type: %s (use 'kustomization' or 'helmrelease')", resource)
	}
}

// flattenFluxResource extracts key fields from a FluxCD resource for clean output.
func flattenFluxResource(obj *unstructured.Unstructured) map[string]interface{} {
	result := map[string]interface{}{
		"name":            obj.GetName(),
		"namespace":       obj.GetNamespace(),
		"uid":             string(obj.GetUID()),
		"resourceVersion": obj.GetResourceVersion(),
		"created":         obj.GetCreationTimestamp().Time.Format(time.RFC3339),
	}

	// Extract spec
	if spec, ok := obj.Object["spec"].(map[string]interface{}); ok {
		result["spec"] = map[string]interface{}{
			"interval":  spec["interval"],
			"path":      spec["path"],
			"prune":     spec["prune"],
			"suspended": spec["suspend"],
		}
		if sourceRef, ok := spec["sourceRef"].(map[string]interface{}); ok {
			result["sourceRef"] = sourceRef
		}
	}

	// Extract status
	if status, ok := obj.Object["status"].(map[string]interface{}); ok {
		result["status"] = map[string]interface{}{
			"observedGeneration":  status["observedGeneration"],
			"lastAppliedRevision": status["lastAppliedRevision"],
			"conditions":          status["conditions"],
		}
	}

	return result
}

// analyzeFluxHealth counts ready/not-ready/suspended resources.
func analyzeFluxHealth(items []unstructured.Unstructured) (result struct {
	Kind      string `json:"kind"`
	Total     int    `json:"total"`
	Ready     int    `json:"ready"`
	NotReady  int    `json:"not_ready"`
	Suspended int    `json:"suspended"`
}) {
	result.Total = len(items)
	for _, item := range items {
		// Check suspend
		if spec, ok := item.Object["spec"].(map[string]interface{}); ok {
			if suspended, ok := spec["suspend"].(bool); ok && suspended {
				result.Suspended++
				continue
			}
		}

		// Check ready condition
		conditions, _ := getNestedSlice(item.Object, "status", "conditions")
		ready := false
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" && cm["status"] == "True" {
					ready = true
					break
				}
			}
		}
		if ready {
			result.Ready++
		} else {
			result.NotReady++
		}
	}
	return
}

// ── Unstructured Helpers ──────────────────────────────────────

func getNestedString(obj map[string]interface{}, fields ...string) (string, bool) {
	var val interface{} = obj
	for _, field := range fields {
		if m, ok := val.(map[string]interface{}); ok {
			val = m[field]
		} else {
			return "", false
		}
	}
	if s, ok := val.(string); ok {
		return s, true
	}
	return "", false
}

func getNestedSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool) {
	var val interface{} = obj
	for _, field := range fields {
		if m, ok := val.(map[string]interface{}); ok {
			val = m[field]
		} else {
			return nil, false
		}
	}
	if s, ok := val.([]interface{}); ok {
		return s, true
	}
	return nil, false
}

// formatAge returns a human-readable age string.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd%dh", days, int(d.Hours())%24)
	}
}

// ── v2 Actions: History, Resource Tree, Events, Logs ─────────

// historyActionParams is the input for the history action.
type historyActionParams struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // "Kustomization" or "HelmRelease"
	Limit     int    `json:"limit,omitempty"`
}

// HistoryEntry represents a reconciliation history entry.
type HistoryEntry struct {
	Revision    string    `json:"revision"`
	Status      string    `json:"status"`
	Message     string    `json:"message"`
	LastTransition time.Time `json:"last_transition_time"`
}

func (p *FluxCDPlugin) history(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	var input historyActionParams
	if err := actionInput(params, &input); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if input.Name == "" || input.Namespace == "" {
		return nil, fmt.Errorf("name and namespace are required")
	}
	if input.Limit <= 0 {
		input.Limit = 10
	}

	gvr := kustomizationGVR
	if strings.EqualFold(input.Kind, "HelmRelease") {
		gvr = helmReleaseGVR
	}

	obj, err := c.client.Resource(gvr).Namespace(input.Namespace).Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get resource: %w", err)
	}

	// Extract status.conditions for reconciliation history
	entries := extractHistoryFromConditions(obj)

	// Limit results
	if len(entries) > input.Limit {
		entries = entries[:input.Limit]
	}

	return actionOutput(entries)
}

// extractHistoryFromConditions extracts reconciliation history from Flux conditions.
func extractHistoryFromConditions(obj *unstructured.Unstructured) []HistoryEntry {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return nil
	}

	var entries []HistoryEntry
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		entry := HistoryEntry{
			Revision: getStringField(obj.Object, "status", "lastAttemptedRevision"),
			Message:  fmt.Sprintf("%v", condMap["message"]),
		}
		if status, ok := condMap["status"].(string); ok {
			entry.Status = status
		}
		if t, ok := condMap["lastTransitionTime"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				entry.LastTransition = parsed
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// resourceTreeActionParams is the input for the resource_tree action.
type resourceTreeActionParams struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
}

// ResourceNode represents a resource in the tree.
type ResourceNode struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Status     string `json:"status"`
	UID        string `json:"uid,omitempty"`
}

func (p *FluxCDPlugin) resourceTree(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	var input resourceTreeActionParams
	if err := actionInput(params, &input); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if input.Name == "" || input.Namespace == "" {
		return nil, fmt.Errorf("name and namespace are required")
	}

	gvr := kustomizationGVR
	if strings.EqualFold(input.Kind, "HelmRelease") {
		gvr = helmReleaseGVR
	}

	obj, err := c.client.Resource(gvr).Namespace(input.Namespace).Get(ctx, input.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get resource: %w", err)
	}

	// Build tree: root node + owned resources via label selector
	nodes := []ResourceNode{
		{
			APIVersion: obj.GetAPIVersion(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Status:     singleResourceStatus(obj),
			UID:        string(obj.GetUID()),
		},
	}

	// Find owned resources via kustomize.toolkit.fluxcd.io/name or helm.toolkit.fluxcd.io/name labels
	labelSelector := fmt.Sprintf("kustomize.toolkit.fluxcd.io/name=%s", input.Name)
	if strings.EqualFold(input.Kind, "HelmRelease") {
		labelSelector = fmt.Sprintf("helm.toolkit.fluxcd.io/name=%s", input.Name)
	}

	// Search common resource types for owned resources
	ownedGVRs := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "", Version: "v1", Resource: "serviceaccounts"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
	}

	for _, g := range ownedGVRs {
		list, err := c.client.Resource(g).Namespace(input.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			continue // Skip resources that don't exist or aren't accessible
		}
		for _, item := range list.Items {
			status := "Unknown"
			if conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); found && len(conditions) > 0 {
				status = "Healthy"
			}
			nodes = append(nodes, ResourceNode{
				APIVersion: item.GetAPIVersion(),
				Kind:       item.GetKind(),
				Name:       item.GetName(),
				Namespace:  item.GetNamespace(),
				Status:     status,
				UID:        string(item.GetUID()),
			})
		}
	}

	return actionOutput(nodes)
}

// eventsActionParams is the input for the events action.
type eventsActionParams struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Limit     int    `json:"limit,omitempty"`
}

// K8sEvent represents a Kubernetes event.
type K8sEvent struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
	Count     int32     `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

func (p *FluxCDPlugin) events(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	var input eventsActionParams
	if err := actionInput(params, &input); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if input.Name == "" || input.Namespace == "" {
		return nil, fmt.Errorf("name and namespace are required")
	}
	if input.Limit <= 0 {
		input.Limit = 50
	}

	// Use typed client for events
	clientset, err := kubernetes.NewForConfig(c.restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	fieldSelector := fmt.Sprintf("involvedObject.name=%s", input.Name)
	if input.Kind != "" {
		fieldSelector += fmt.Sprintf(",involvedObject.kind=%s", input.Kind)
	}

	eventList, err := clientset.CoreV1().Events(input.Namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fieldSelector,
		Limit:         int64(input.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	events := make([]K8sEvent, 0, len(eventList.Items))
	for _, e := range eventList.Items {
		events = append(events, K8sEvent{
			Type:      e.Type,
			Reason:    e.Reason,
			Message:   e.Message,
			Source:    e.Source.Component,
			Count:     e.Count,
			FirstSeen: e.FirstTimestamp.Time,
			LastSeen:  e.LastTimestamp.Time,
		})
	}

	return actionOutput(events)
}

// logsActionParams is the input for the logs action.
type logsActionParams struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Container  string `json:"container,omitempty"`
	TailLines  int64  `json:"tail_lines,omitempty"`
	Follow     bool   `json:"follow,omitempty"`
}

// LogEntry represents a log line.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Container string `json:"container,omitempty"`
}

func (p *FluxCDPlugin) logs(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	var input logsActionParams
	if err := actionInput(params, &input); err != nil {
		return nil, fmt.Errorf("parse params: %w", err)
	}
	if input.Name == "" || input.Namespace == "" {
		return nil, fmt.Errorf("name and namespace are required")
	}
	if input.TailLines <= 0 {
		input.TailLines = 100
	}

	clientset, err := kubernetes.NewForConfig(c.restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	// Find pods owned by this resource
	labelSelector := fmt.Sprintf("kustomize.toolkit.fluxcd.io/name=%s", input.Name)
	if strings.EqualFold(input.Kind, "HelmRelease") {
		labelSelector = fmt.Sprintf("helm.toolkit.fluxcd.io/name=%s", input.Name)
	}

	podList, err := clientset.CoreV1().Pods(input.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	var allLogs []LogEntry
	for _, pod := range podList.Items {
		for _, container := range pod.Spec.Containers {
			if input.Container != "" && container.Name != input.Container {
				continue
			}
			logOpts := &corev1.PodLogOptions{
				Container:  container.Name,
				TailLines:  &input.TailLines,
				Timestamps: true,
			}
			req := clientset.CoreV1().Pods(input.Namespace).GetLogs(pod.Name, logOpts)
			stream, err := req.Stream(ctx)
			if err != nil {
				continue // Skip containers that fail
			}
			buf := make([]byte, 64*1024)
			n, _ := stream.Read(buf)
			_ = stream.Close()
			if n > 0 {
				lines := strings.Split(string(buf[:n]), "\n")
				for _, line := range lines {
					if line == "" {
						continue
					}
					entry := LogEntry{Container: container.Name}
					// Parse timestamp from log line (format: 2024-01-01T00:00:00.000000000Z message)
					if idx := strings.Index(line, " "); idx > 0 {
						entry.Timestamp = line[:idx]
						entry.Message = line[idx+1:]
					} else {
						entry.Message = line
					}
					allLogs = append(allLogs, entry)
				}
			}
		}
	}

	return actionOutput(allLogs)
}

// PluginCapabilities describes what the FluxCD plugin supports.
type PluginCapabilities struct {
	EngineType    string   `json:"engine_type"`
	SupportedKinds []string `json:"supported_kinds"`
	Diff          bool     `json:"diff"`
	History       bool     `json:"history"`
	ResourceTree  bool     `json:"resource_tree"`
	Events        bool     `json:"events"`
	Logs          bool     `json:"logs"`
	Refresh       bool     `json:"refresh"`
	AutoSync      bool     `json:"auto_sync"`
	Projects      bool     `json:"projects"`
}

func (p *FluxCDPlugin) capabilities(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	caps := PluginCapabilities{
		EngineType:     "fluxcd",
		SupportedKinds: []string{"Kustomization", "HelmRelease"},
		Diff:           false, // FluxCD doesn't have a native diff API
		History:        true,
		ResourceTree:   true,
		Events:         true,
		Logs:           true,
		Refresh:        true,
		AutoSync:       true, // via suspend/resume
		Projects:       false, // FluxCD doesn't have AppProjects
	}
	return actionOutput(caps)
}

// listGitRepositories lists GitRepository sources in the cluster.
type listGitReposParams struct {
	Namespace string `json:"namespace,omitempty"`
}

func (p *FluxCDPlugin) listGitRepositories(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	var input listGitReposParams
	if len(params) > 0 {
		if err := actionInput(params, &input); err != nil {
			return nil, fmt.Errorf("parse params: %w", err)
		}
	}

	var list *unstructured.UnstructuredList
	var err error
	if input.Namespace != "" {
		list, err = c.client.Resource(gitRepositoryGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.client.Resource(gitRepositoryGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list git repositories: %w", err)
	}

	items := make([]map[string]interface{}, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, flattenFluxResource(&item))
	}

	return actionOutput(items)
}

// listHelmRepositories lists HelmRepository sources in the cluster.
type listHelmReposParams struct {
	Namespace string `json:"namespace,omitempty"`
}

func (p *FluxCDPlugin) listHelmRepositories(ctx context.Context, c *FluxController, params []byte) ([]byte, error) {
	var input listHelmReposParams
	if len(params) > 0 {
		if err := actionInput(params, &input); err != nil {
			return nil, fmt.Errorf("parse params: %w", err)
		}
	}

	var list *unstructured.UnstructuredList
	var err error
	if input.Namespace != "" {
		list, err = c.client.Resource(helmRepositoryGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.client.Resource(helmRepositoryGVR).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list helm repositories: %w", err)
	}

	items := make([]map[string]interface{}, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, flattenFluxResource(&item))
	}

	return actionOutput(items)
}

// singleResourceStatus returns a simple health status string for a single Flux resource.
func singleResourceStatus(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || len(conditions) == 0 {
		return "Unknown"
	}
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		if condMap["type"] == "Ready" {
			if status, ok := condMap["status"].(string); ok {
				if status == "True" {
					return "Healthy"
				}
				return "Degraded"
			}
		}
	}
	// Check if suspended
	suspended, _, _ := unstructured.NestedBool(obj.Object, "spec", "suspend")
	if suspended {
		return "Suspended"
	}
	return "Progressing"
}

// getStringField is a helper to extract nested string fields from unstructured objects.
func getStringField(obj map[string]interface{}, fields ...string) string {
	current := obj
	for i, field := range fields {
		if i == len(fields)-1 {
			if val, ok := current[field].(string); ok {
				return val
			}
			return ""
		}
		next, ok := current[field].(map[string]interface{})
		if !ok {
			return ""
		}
		current = next
	}
	return ""
}

// Ensure json import is used
var _ = json.Marshal
