package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ChartSpec describes a Helm chart to deploy.
type ChartSpec struct {
	SourceType   string `json:"source_type"`
	ChartURL     string `json:"chart_url"`
	ChartName    string `json:"chart_name"` // chart package name (for repo-based installs)
	ChartVersion string `json:"chart_version"`
	ChartPath    string `json:"chart_path"`
}

// DeploySpec describes what to deploy to a cluster.
type DeploySpec struct {
	ReleaseName string            `json:"release_name"`
	Namespace   string            `json:"namespace"`
	Replicas    int32             `json:"replicas"`
	Containers  []ContainerSpec   `json:"containers"`
	Service     *ServiceSpec      `json:"service,omitempty"`
	ValuesYAML  string            `json:"values_yaml,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Chart       *ChartSpec        `json:"chart,omitempty"`
}

// ContainerSpec describes a container to deploy.
type ContainerSpec struct {
	Name   string  `json:"name"`
	Image  string  `json:"image"`
	CPU    string  `json:"cpu"`
	Memory string  `json:"memory"`
	Ports  []int32 `json:"ports"`
}

// ServiceSpec describes a Kubernetes Service.
type ServiceSpec struct {
	Port int32  `json:"port"`
	Type string `json:"type"`
}

// DeployResult contains the outcome of a deployment.
type DeployResult struct {
	ReleaseName string `json:"release_name"`
	Namespace   string `json:"namespace"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// Deploy creates or updates a Kubernetes Deployment and optional Service.
func (c *Client) Deploy(ctx context.Context, spec DeploySpec) (*DeployResult, error) {
	if spec.Namespace == "" {
		spec.Namespace = "default"
	}
	if spec.Replicas <= 0 {
		spec.Replicas = 1
	}
	if spec.ReleaseName == "" {
		return nil, fmt.Errorf("release_name is required")
	}

	// Ensure namespace exists
	if err := c.ensureNamespace(ctx, spec.Namespace); err != nil {
		log.Printf("WARN: ensure namespace %q: %v", spec.Namespace, err)
		// Continue anyway — namespace might already exist or be auto-created
	}

	// Require at least one container — do NOT silently default to nginx
	if len(spec.Containers) == 0 {
		return nil, fmt.Errorf("no containers specified in deployment spec — at least one container with an image is required")
	}

	// Build Deployment
	deploy := c.buildDeployment(spec)

	// Apply Deployment
	deploymentsClient := c.clientset.AppsV1().Deployments(spec.Namespace)
	existing, err := deploymentsClient.Get(ctx, deploy.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = deploymentsClient.Create(ctx, deploy, metav1.CreateOptions{})
			if err != nil {
				return nil, fmt.Errorf("create deployment %s/%s: %w", spec.Namespace, deploy.Name, err)
			}
			log.Printf("Created deployment %s/%s", spec.Namespace, deploy.Name)
		} else {
			return nil, fmt.Errorf("get deployment %s/%s: %w", spec.Namespace, deploy.Name, err)
		}
	} else {
		// Update existing
		deploy.ResourceVersion = existing.ResourceVersion
		_, err = deploymentsClient.Update(ctx, deploy, metav1.UpdateOptions{})
		if err != nil {
			return nil, fmt.Errorf("update deployment %s/%s: %w", spec.Namespace, deploy.Name, err)
		}
		log.Printf("Updated deployment %s/%s", spec.Namespace, deploy.Name)
	}

	// Create Service if specified
	if spec.Service != nil && spec.Service.Port > 0 {
		svc := c.buildService(spec)
		servicesClient := c.clientset.CoreV1().Services(spec.Namespace)
		existingSvc, err := servicesClient.Get(ctx, svc.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				_, err = servicesClient.Create(ctx, svc, metav1.CreateOptions{})
				if err != nil {
					return nil, fmt.Errorf("create service %s/%s: %w", spec.Namespace, svc.Name, err)
				}
				log.Printf("Created service %s/%s", spec.Namespace, svc.Name)
			} else {
				return nil, fmt.Errorf("get service %s/%s: %w", spec.Namespace, svc.Name, err)
			}
		} else {
			svc.ResourceVersion = existingSvc.ResourceVersion
			// Preserve ClusterIP on update
			svc.Spec.ClusterIP = existingSvc.Spec.ClusterIP
			_, err = servicesClient.Update(ctx, svc, metav1.UpdateOptions{})
			if err != nil {
				return nil, fmt.Errorf("update service %s/%s: %w", spec.Namespace, svc.Name, err)
			}
			log.Printf("Updated service %s/%s", spec.Namespace, svc.Name)
		}
	}

	return &DeployResult{
		ReleaseName: spec.ReleaseName,
		Namespace:   spec.Namespace,
		Status:      "deployed",
		Message:     fmt.Sprintf("Successfully deployed %s to %s", spec.ReleaseName, spec.Namespace),
	}, nil
}

// ensureNamespace creates the namespace if it doesn't exist.
func (c *Client) ensureNamespace(ctx context.Context, namespace string) error {
	if namespace == "default" {
		return nil // default always exists
	}
	nsClient := c.clientset.CoreV1().Namespaces()
	_, err := nsClient.Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_, err = nsClient.Create(ctx, ns, metav1.CreateOptions{})
			if err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("create namespace %s: %w", namespace, err)
			}
			log.Printf("Created namespace %s", namespace)
		} else {
			return fmt.Errorf("get namespace %s: %w", namespace, err)
		}
	}
	return nil
}

// buildDeployment constructs an appsv1.Deployment from the spec.
func (c *Client) buildDeployment(spec DeploySpec) *appsv1.Deployment {
	labels := map[string]string{
		"app.kubernetes.io/name":       spec.ReleaseName,
		"app.kubernetes.io/managed-by": "pepa",
	}
	for k, v := range spec.Labels {
		labels[k] = v
	}

	var containers []corev1.Container
	for _, cs := range spec.Containers {
		container := corev1.Container{
			Name:  cs.Name,
			Image: cs.Image,
		}

		// Resource requests/limits
		if cs.CPU != "" || cs.Memory != "" {
			requests := corev1.ResourceList{}
			limits := corev1.ResourceList{}
			if cs.CPU != "" {
				cpu := cs.CPU
				if !strings.HasSuffix(cpu, "m") {
					cpu = cpu + "m"
				}
				q := resource.MustParse(cpu)
				requests[corev1.ResourceCPU] = q
				limits[corev1.ResourceCPU] = q
			}
			if cs.Memory != "" {
				mem := cs.Memory
				if !strings.Contains(mem, "Mi") && !strings.Contains(mem, "Gi") {
					mem = mem + "Mi"
				}
				q := resource.MustParse(mem)
				requests[corev1.ResourceMemory] = q
				limits[corev1.ResourceMemory] = q
			}
			container.Resources = corev1.ResourceRequirements{
				Requests: requests,
				Limits:   limits,
			}
		}

		// Ports
		for _, p := range cs.Ports {
			container.Ports = append(container.Ports, corev1.ContainerPort{
				ContainerPort: p,
				Protocol:      corev1.ProtocolTCP,
			})
		}

		containers = append(containers, container)
	}

	// If no containers after processing, skip (should not happen due to earlier check)
	if len(containers) == 0 {
		containers = []corev1.Container{
			{
				Name:    spec.ReleaseName,
				Image:   "busybox:latest",
				Command: []string{"sh", "-c", "echo 'No image specified' && sleep 3600"},
			},
		}
	}

	replicas := spec.Replicas

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.ReleaseName,
			Namespace: spec.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"github.com/your-username/pepa/managed-by": "pepa",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": spec.ReleaseName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: containers,
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
			},
		},
	}

	return deploy
}

// buildService constructs a corev1.Service from the spec.
func (c *Client) buildService(spec DeploySpec) *corev1.Service {
	labels := map[string]string{
		"app.kubernetes.io/name":       spec.ReleaseName,
		"app.kubernetes.io/managed-by": "pepa",
	}

	svcType := corev1.ServiceTypeClusterIP
	if spec.Service != nil && strings.EqualFold(spec.Service.Type, "LoadBalancer") {
		svcType = corev1.ServiceTypeLoadBalancer
	} else if spec.Service != nil && strings.EqualFold(spec.Service.Type, "NodePort") {
		svcType = corev1.ServiceTypeNodePort
	}

	var ports []corev1.ServicePort
	if spec.Service != nil && spec.Service.Port > 0 {
		ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       spec.Service.Port,
				TargetPort: intstr.FromInt(int(spec.Service.Port)),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	} else if len(spec.Containers) > 0 && len(spec.Containers[0].Ports) > 0 {
		// Use first container port
		ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       spec.Containers[0].Ports[0],
				TargetPort: intstr.FromInt(int(spec.Containers[0].Ports[0])),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.ReleaseName,
			Namespace: spec.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"github.com/your-username/pepa/managed-by": "pepa",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: map[string]string{"app.kubernetes.io/name": spec.ReleaseName},
			Ports:    ports,
		},
	}

	return svc
}

// ParseDeploySpec parses a deployment spec from the JSON stored in the deployments table.
func ParseDeploySpec(specJSON json.RawMessage, releaseName, namespace string, replicas int32) (DeploySpec, error) {
	var raw struct {
		Containers []struct {
			Name   string `json:"name"`
			Image  string `json:"image"`
			CPU    string `json:"cpu"`
			Memory string `json:"memory"`
			Ports  []struct {
				ContainerPort int32 `json:"containerPort"`
			} `json:"ports"`
		} `json:"containers"`
		ValuesYAML string `json:"values_yaml"`
		Service    *struct {
			Port int32  `json:"port"`
			Type string `json:"type"`
		} `json:"service"`
		Chart *struct {
			SourceType   string `json:"source_type"`
			ChartURL     string `json:"chart_url"`
			ChartName    string `json:"chart_name"`
			ChartVersion string `json:"chart_version"`
			ChartPath    string `json:"chart_path"`
		} `json:"chart"`
	}

	if err := json.Unmarshal(specJSON, &raw); err != nil {
		return DeploySpec{}, fmt.Errorf("parse spec: %w", err)
	}

	ds := DeploySpec{
		ReleaseName: releaseName,
		Namespace:   namespace,
		Replicas:    replicas,
		ValuesYAML:  raw.ValuesYAML,
	}

	for _, c := range raw.Containers {
		cs := ContainerSpec{
			Name:   c.Name,
			Image:  c.Image,
			CPU:    c.CPU,
			Memory: c.Memory,
		}
		for _, p := range c.Ports {
			cs.Ports = append(cs.Ports, p.ContainerPort)
		}
		ds.Containers = append(ds.Containers, cs)
	}

	if raw.Service != nil {
		ds.Service = &ServiceSpec{
			Port: raw.Service.Port,
			Type: raw.Service.Type,
		}
	}

	if raw.Chart != nil {
		chartName := raw.Chart.ChartName
		if chartName == "" {
			chartName = raw.Chart.ChartPath // fallback: chart_path used as chart name
		}
		ds.Chart = &ChartSpec{
			SourceType:   raw.Chart.SourceType,
			ChartURL:     raw.Chart.ChartURL,
			ChartName:    chartName,
			ChartVersion: raw.Chart.ChartVersion,
			ChartPath:    raw.Chart.ChartPath,
		}
	}

	return ds, nil
}
