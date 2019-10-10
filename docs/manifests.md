How to write Kubernetes application manifests
=============================================

Directory tree
--------------

```console
.
├── argocd-config # Argo CD CRD based app configurations
│   ├── base
│   │   └── monitoring.yaml # CRD yaml for app "monitoring" configuration includes repository URL and path.
│   └── overlays
│       ├── bk
│       ├── prod
│       └── stage
│           ├── kustumization.yaml # Argo CD CRD deployment for stage.
│           └── monitoring.yaml    # overlays for base/monitoring.yaml.
├─── monitoring # App "monitoring" deployment manifests.
|   ├── base
|   │   ├── deployment.yaml    # Plain manifest files of each K8s object name
|   │   ├── kustomization.yaml
|   │   └── service.yaml
|   └── overlays
|       ├── dev
|       ├── prod
|       └── stage
|           ├── cpu_count.yaml     # Some tuning
|           ├── kustomization.yaml
|           └── proxy.yaml         # NO_PROXY, HTTP_PROXY, HTTPS_PROXY environment variables
└────── test                       # Ginkgo based deployment test
...
```

`argocd-config/overlays/stage/kustomization.yaml`

```yaml
bases: # It includes all applications for stage.
- ../../base
...

patches:
- monitoring.yaml # Argo CD CRD of app "monitoring" for stage.
```

`argocd-config/overlays/stage/monitoring.yaml`

```yaml
# Custom Resource Definition for Argo CD app "monitoring"
spec:
  project: default
  source:
    repoURL: https://github.com/cybozu-go/neco-apps.git
    targetRevision: release         # branch name
    path: monitoring/overlays/stage # Path to Kustomize based app path
    kustomize:
      namePrefix: stage-
  destination:
    server: https://kubernetes.default.svc
    namespace: default
```

`monitoring/overlays/stage/kustomization.yaml`

```yaml
bases:   # It includes all K8s objects for monitoring.
- ../../base
patches: # Patches for stage
- cpu_count.yaml
- proxy.yaml
```

`monitoring/base/kustomization.yaml`

```yaml
resources:   # It includes all K8s objects for monitoring.
- deployment.yaml
- service.yaml
```
