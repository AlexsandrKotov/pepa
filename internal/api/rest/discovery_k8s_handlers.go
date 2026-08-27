package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pepa/pepa/internal/auth"
)

// =============================================================================
// Kubernetes Deployment Management
// =============================================================================

// DeploymentInfo contains detailed deployment information for the UI
type DeploymentInfo struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Cluster           string            `json:"cluster"`
	Replicas          int32             `json:"replicas"`
	ReadyReplicas     int32             `json:"ready_replicas"`
	AvailableReplicas int32             `json:"available_replicas"`
	Image             string            `json:"image"`
	Images            []string          `json:"images"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	Strategy          string            `json:"strategy"`
	CreatedAt         string            `json:"created_at"`
	Env               map[string]string `json:"env,omitempty"`
	ResourceLimits    map[string]string `json:"resource_limits,omitempty"`
	ResourceRequests  map[string]string `json:"resource_requests,omitempty"`
}

// k8sGetDeployment returns detailed deployment information
func k8sGetDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		info, err := getDeploymentInfo(ctx, kubeconfig, namespace, name, clusterName)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, info)
	}
}

// k8sUpdateDeployment updates deployment spec (image, env, resources)
func k8sUpdateDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var body struct {
			Image    *string           `json:"image,omitempty"`
			Env      map[string]string `json:"env,omitempty"`
			Replicas *int32            `json:"replicas,omitempty"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		err = updateDeployment(ctx, kubeconfig, namespace, name, body.Image, body.Env, body.Replicas)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "update", "k8s_deployment", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName, "namespace": namespace, "name": name})

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Deployment %s/%s updated", namespace, name),
		})
	}
}

// k8sScaleDeployment scales a deployment to the specified replicas
func k8sScaleDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		var body struct {
			Replicas int32 `json:"replicas"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		err = scaleDeployment(ctx, kubeconfig, namespace, name, body.Replicas)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "scale", "k8s_deployment", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName, "replicas": body.Replicas})

		c.JSON(http.StatusOK, gin.H{
			"message":  fmt.Sprintf("Deployment %s/%s scaled to %d replicas", namespace, name, body.Replicas),
			"replicas": body.Replicas,
		})
	}
}

// k8sRestartDeployment triggers a rolling restart
func k8sRestartDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = restartDeployment(ctx, kubeconfig, namespace, name)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "restart", "k8s_deployment", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName})

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Deployment %s/%s restart triggered", namespace, name),
		})
	}
}

// k8sDeleteDeployment deletes a deployment
func k8sDeleteDeployment(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = deleteDeployment(ctx, kubeconfig, namespace, name)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		logAudit(deps, c, "delete", "k8s_deployment", fmt.Sprintf("%s/%s/%s", clusterName, namespace, name), nil, gin.H{"cluster": clusterName})

		c.JSON(http.StatusOK, gin.H{
			"message": fmt.Sprintf("Deployment %s/%s deleted", namespace, name),
		})
	}
}

// k8sGetLogs returns container logs from a deployment's pods
func k8sGetLogs(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")
		container := c.Query("container")
		tailLines := int64(100)
		if t := c.Query("tail"); t != "" {
			if parsed, parseErr := strconv.ParseInt(t, 10, 64); parseErr == nil {
				tailLines = parsed
			}
		}

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		logs, err := getPodLogs(ctx, kubeconfig, namespace, name, container, tailLines)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"logs":      logs,
			"name":      name,
			"namespace": namespace,
		})
	}
}

// k8sGetEvents returns Kubernetes events for a deployment
func k8sGetEvents(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		clusterName := c.Param("cluster")
		namespace := c.Param("namespace")
		name := c.Param("name")

		ctx := c.Request.Context()
		tenantID := auth.GetTenantID(c)

		kubeconfig, err := getClusterKubeconfig(ctx, deps, tenantID, clusterName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		events, err := getDeploymentEvents(ctx, kubeconfig, namespace, name)
		if err != nil {
			respondInternalError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"events":    events,
			"name":      name,
			"namespace": namespace,
		})
	}
}

// =============================================================================
// K8s Helper Functions
// =============================================================================

func getDeploymentInfo(ctx context.Context, kubeconfig, namespace, name, clusterName string) (*DeploymentInfo, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	apiPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, name)
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
	if err != nil {
		return nil, fmt.Errorf("get deployment: %w", err)
	}

	var deploy map[string]interface{}
	if err := json.Unmarshal(body, &deploy); err != nil {
		return nil, fmt.Errorf("unmarshal deployment: %w", err)
	}

	info := &DeploymentInfo{
		Name:      name,
		Namespace: namespace,
		Cluster:   clusterName,
	}

	// Extract metadata
	metadata, _ := deploy["metadata"].(map[string]interface{})
	if metadata != nil {
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			info.Labels = make(map[string]string)
			for k, v := range labels {
				info.Labels[k] = fmt.Sprintf("%v", v)
			}
		}
		if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
			info.Annotations = make(map[string]string)
			for k, v := range annotations {
				info.Annotations[k] = fmt.Sprintf("%v", v)
			}
		}
		info.CreatedAt, _ = metadata["creationTimestamp"].(string)
	}

	// Extract spec
	spec, _ := deploy["spec"].(map[string]interface{})
	if spec != nil {
		if replicas, ok := spec["replicas"].(float64); ok {
			info.Replicas = int32(replicas)
		}
		if strategy, ok := spec["strategy"].(map[string]interface{}); ok {
			info.Strategy, _ = strategy["type"].(string)
		}

		// Extract container info
		template, _ := spec["template"].(map[string]interface{})
		if template != nil {
			templateSpec, _ := template["spec"].(map[string]interface{})
			if templateSpec != nil {
				containers, _ := templateSpec["containers"].([]interface{})
				for _, ctr := range containers {
					ctrMap, _ := ctr.(map[string]interface{})
					if ctrMap == nil {
						continue
					}
					if img, ok := ctrMap["image"].(string); ok {
						info.Images = append(info.Images, img)
						if info.Image == "" {
							info.Image = img
						}
					}
					// Extract env vars
					if envVars, ok := ctrMap["env"].([]interface{}); ok {
						if info.Env == nil {
							info.Env = make(map[string]string)
						}
						for _, e := range envVars {
							eMap, _ := e.(map[string]interface{})
							if eMap == nil {
								continue
							}
							eName, _ := eMap["name"].(string)
							eValue, _ := eMap["value"].(string)
							if eName != "" {
								info.Env[eName] = eValue
							}
						}
					}
					// Extract resources
					if resources, ok := ctrMap["resources"].(map[string]interface{}); ok {
						if limits, ok := resources["limits"].(map[string]interface{}); ok {
							info.ResourceLimits = make(map[string]string)
							for k, v := range limits {
								info.ResourceLimits[k] = fmt.Sprintf("%v", v)
							}
						}
						if requests, ok := resources["requests"].(map[string]interface{}); ok {
							info.ResourceRequests = make(map[string]string)
							for k, v := range requests {
								info.ResourceRequests[k] = fmt.Sprintf("%v", v)
							}
						}
					}
				}
			}
		}
	}

	// Extract status
	status, _ := deploy["status"].(map[string]interface{})
	if status != nil {
		if ready, ok := status["readyReplicas"].(float64); ok {
			info.ReadyReplicas = int32(ready)
		}
		if avail, ok := status["availableReplicas"].(float64); ok {
			info.AvailableReplicas = int32(avail)
		}
	}

	return info, nil
}

func updateDeployment(ctx context.Context, kubeconfig, namespace, name string, image *string, env map[string]string, replicas *int32) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	// Get current deployment
	apiPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, name)
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	var deploy map[string]interface{}
	if err := json.Unmarshal(body, &deploy); err != nil {
		return fmt.Errorf("unmarshal deployment: %w", err)
	}

	// Modify spec
	spec, _ := deploy["spec"].(map[string]interface{})
	if spec == nil {
		return fmt.Errorf("invalid deployment spec")
	}

	if replicas != nil {
		spec["replicas"] = *replicas
	}

	// Modify template containers
	template, _ := spec["template"].(map[string]interface{})
	if template != nil {
		templateSpec, _ := template["spec"].(map[string]interface{})
		if templateSpec != nil {
			containers, _ := templateSpec["containers"].([]interface{})
			for i, ctr := range containers {
				ctrMap, _ := ctr.(map[string]interface{})
				if ctrMap == nil {
					continue
				}
				if image != nil {
					ctrMap["image"] = *image
				}
				if env != nil {
					// Merge env vars
					existingEnv, _ := ctrMap["env"].([]interface{})
					envMap := make(map[string]interface{})
					for _, e := range existingEnv {
						eMap, _ := e.(map[string]interface{})
						if eMap == nil {
							continue
						}
						eName, _ := eMap["name"].(string)
						envMap[eName] = e
					}
					for k, v := range env {
						envMap[k] = map[string]interface{}{"name": k, "value": v}
					}
					newEnv := make([]interface{}, 0, len(envMap))
					for _, v := range envMap {
						newEnv = append(newEnv, v)
					}
					ctrMap["env"] = newEnv
				}
				containers[i] = ctrMap
			}
			templateSpec["containers"] = containers
		}
	}

	// Patch back
	patchData, err := json.Marshal(deploy)
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}

	_, err = k8sRequestWithBody(ctx, restConfig, "PUT", apiPath, string(patchData))
	if err != nil {
		return fmt.Errorf("update deployment: %w", err)
	}

	return nil
}

func scaleDeployment(ctx context.Context, kubeconfig, namespace, name string, replicas int32) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	apiPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s/scale", namespace, name)
	patchData := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)

	_, err = k8sRequestWithBody(ctx, restConfig, "PATCH", apiPath, patchData)
	if err != nil {
		return fmt.Errorf("scale deployment: %w", err)
	}

	return nil
}

func restartDeployment(ctx context.Context, kubeconfig, namespace, name string) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	// Get current deployment to get the annotation
	apiPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, name)
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", apiPath, "")
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	var deploy map[string]interface{}
	if err := json.Unmarshal(body, &deploy); err != nil {
		return fmt.Errorf("unmarshal deployment: %w", err)
	}

	// Add restart annotation
	metadata, _ := deploy["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	annotations, _ := metadata["annotations"].(map[string]interface{})
	if annotations == nil {
		annotations = map[string]interface{}{}
	}
	annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339)
	metadata["annotations"] = annotations

	patchData, err := json.Marshal(map[string]interface{}{
		"metadata": metadata,
	})
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}

	_, err = k8sRequestWithBody(ctx, restConfig, "PATCH", apiPath, string(patchData))
	if err != nil {
		return fmt.Errorf("restart deployment: %w", err)
	}

	return nil
}

func deleteDeployment(ctx context.Context, kubeconfig, namespace, name string) error {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build rest config: %w", err)
	}

	apiPath := fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", namespace, name)
	_, err = k8sRequestWithBody(ctx, restConfig, "DELETE", apiPath, "")
	if err != nil {
		return fmt.Errorf("delete deployment: %w", err)
	}

	return nil
}

func getPodLogs(ctx context.Context, kubeconfig, namespace, deployName, container string, tailLines int64) (string, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return "", fmt.Errorf("build rest config: %w", err)
	}

	// First, find a pod for this deployment
	podsPath := fmt.Sprintf("/api/v1/namespaces/%s/pods?labelSelector=app=%s&limit=1", namespace, deployName)
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", podsPath, "")
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}

	var pods map[string]interface{}
	if err := json.Unmarshal(body, &pods); err != nil {
		return "", fmt.Errorf("unmarshal pods: %w", err)
	}

	items, _ := pods["items"].([]interface{})
	if len(items) == 0 {
		// Try with different label selector
		podsPath = fmt.Sprintf("/api/v1/namespaces/%s/pods?limit=5", namespace)
		body, err = k8sRequestWithBody(ctx, restConfig, "GET", podsPath, "")
		if err != nil {
			return "", fmt.Errorf("list pods: %w", err)
		}
		if err := json.Unmarshal(body, &pods); err != nil {
			return "", fmt.Errorf("unmarshal pods: %w", err)
		}
		items, _ = pods["items"].([]interface{})
		if len(items) == 0 {
			return "No pods found", nil
		}
	}

	// Get first pod name
	pod, _ := items[0].(map[string]interface{})
	podMeta, _ := pod["metadata"].(map[string]interface{})
	podName, _ := podMeta["name"].(string)
	if podName == "" {
		return "Could not determine pod name", nil
	}

	// If no container specified, try to find the first one from pod spec
	if container == "" {
		podSpec, _ := pod["spec"].(map[string]interface{})
		if podSpec != nil {
			containers, _ := podSpec["containers"].([]interface{})
			if len(containers) > 0 {
				firstCtr, _ := containers[0].(map[string]interface{})
				if firstCtr != nil {
					container, _ = firstCtr["name"].(string)
				}
			}
		}
	}

	// Get logs
	logsPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log?tailLines=%d", namespace, podName, tailLines)
	if container != "" {
		logsPath += "&container=" + container
	}

	logBody, err := k8sRequestWithBody(ctx, restConfig, "GET", logsPath, "")
	if err != nil {
		return "", fmt.Errorf("get logs: %w", err)
	}

	return string(logBody), nil
}

func getDeploymentEvents(ctx context.Context, kubeconfig, namespace, name string) ([]map[string]interface{}, error) {
	restConfig, err := buildRestConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}

	// Get events for the deployment
	eventsPath := fmt.Sprintf("/api/v1/namespaces/%s/events?fieldSelector=involvedObject.name=%s,involvedObject.kind=Deployment", namespace, name)
	body, err := k8sRequestWithBody(ctx, restConfig, "GET", eventsPath, "")
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}

	var eventsList map[string]interface{}
	if err := json.Unmarshal(body, &eventsList); err != nil {
		return nil, fmt.Errorf("unmarshal events: %w", err)
	}

	items, _ := eventsList["items"].([]interface{})
	var events []map[string]interface{}
	for _, item := range items {
		event, _ := item.(map[string]interface{})
		if event == nil {
			continue
		}
		simplified := map[string]interface{}{
			"type":           event["type"],
			"reason":         event["reason"],
			"message":        event["message"],
			"count":          event["count"],
			"lastTimestamp":  event["lastTimestamp"],
			"firstTimestamp": event["firstTimestamp"],
		}
		events = append(events, simplified)
	}

	return events, nil
}
