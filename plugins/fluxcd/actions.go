package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
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
)

// FluxController handles FluxCD CRD operations via dynamic Kubernetes client.
type FluxController struct {
	client dynamic.Interface
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

	return &FluxController{client: client}, nil
}

// ── Kustomization Actions ─────────────────────────────────────

func (p *FluxCDPlugin) listKustomizations(ctx context.Context, fc *FluxController, params []byte) ([]byte, error) {
	var input struct {
		Namespace string `json:"namespace"`
	}
	if err := actionInput(params, &input); err != nil {
		return nil, err
	}
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	list, err := fc.client.Resource(kustomizationGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
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
	if input.Namespace == "" {
		input.Namespace = "flux-system"
	}

	list, err := fc.client.Resource(helmReleaseGVR).Namespace(input.Namespace).List(ctx, metav1.ListOptions{})
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
