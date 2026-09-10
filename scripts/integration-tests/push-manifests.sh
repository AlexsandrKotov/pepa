#!/bin/bash
set -e

GITEA="http://localhost:3001/api/v1"
AUTH="pepa:PepaTest2026!"

push_file() {
  local repo="$1" path="$2" content="$3" msg="$4"
  local encoded
  encoded=$(echo -n "$content" | base64)
  
  # Check if file exists
  local code
  code=$(curl -s -o /dev/null -w "%{http_code}" "$GITEA/repos/$repo/contents/$path" -u "$AUTH")
  
  if [ "$code" = "200" ]; then
    # Update - need sha
    local sha
    sha=$(curl -s "$GITEA/repos/$repo/contents/$path" -u "$AUTH" | jq -r '.sha')
    curl -s -o /dev/null -w "  %{http_code}" -X PUT "$GITEA/repos/$repo/contents/$path" \
      -u "$AUTH" -H "Content-Type: application/json" \
      -d "{\"message\":\"$msg\",\"content\":\"$encoded\",\"sha\":\"$sha\",\"author\":{\"name\":\"PEPA\",\"email\":\"pepa@local\"}}"
    echo " updated: $path"
  else
    # Create
    curl -s -o /dev/null -w "  %{http_code}" -X POST "$GITEA/repos/$repo/contents/$path" \
      -u "$AUTH" -H "Content-Type: application/json" \
      -d "{\"message\":\"$msg\",\"content\":\"$encoded\",\"author\":{\"name\":\"PEPA\",\"email\":\"pepa@local\"}}"
    echo " created: $path"
  fi
}

echo "=== FluxCD manifests (pepa-test/fluxcd-manifests) ==="
REPO="pepa-test/fluxcd-manifests"

push_file "$REPO" "kustomization.yaml" \
"apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - nginx-deployment.yaml
  - nginx-service.yaml
" "Add root kustomization"

push_file "$REPO" "nginx-deployment.yaml" \
"apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-flux
  namespace: pepa-e2e
  labels:
    app: nginx-flux
    managed-by: fluxcd
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx-flux
  template:
    metadata:
      labels:
        app: nginx-flux
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        ports:
        - containerPort: 80
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
" "Add nginx deployment"

push_file "$REPO" "nginx-service.yaml" \
"apiVersion: v1
kind: Service
metadata:
  name: nginx-flux
  namespace: pepa-e2e
spec:
  selector:
    app: nginx-flux
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
" "Add nginx service"

echo ""
echo "=== ArgoCD manifests (pepa-test/argocd-manifests) ==="
REPO="pepa-test/argocd-manifests"

push_file "$REPO" "kustomization.yaml" \
"apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - nginx-deployment.yaml
  - nginx-service.yaml
" "Add root kustomization"

push_file "$REPO" "nginx-deployment.yaml" \
"apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-argocd
  namespace: pepa-e2e
  labels:
    app: nginx-argocd
    managed-by: argocd
spec:
  replicas: 2
  selector:
    matchLabels:
      app: nginx-argocd
  template:
    metadata:
      labels:
        app: nginx-argocd
    spec:
      containers:
      - name: nginx
        image: nginx:1.27-alpine
        ports:
        - containerPort: 80
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
" "Add nginx deployment"

push_file "$REPO" "nginx-service.yaml" \
"apiVersion: v1
kind: Service
metadata:
  name: nginx-argocd
  namespace: pepa-e2e
spec:
  selector:
    app: nginx-argocd
  ports:
  - port: 80
    targetPort: 80
  type: ClusterIP
" "Add nginx service"

echo ""
echo "=== Done ==="
