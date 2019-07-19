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
└── monitoring # App "monitoring" deployment manifests.
    ├── base
    │   ├── deployment.yaml    # Plain manifest files of each K8s object name
    │   ├── kustomization.yaml
    │   └── service.yaml
    ├── overlays
    │   ├── dev
    │   ├── prod
    │   └── stage
    │       ├── cpu_count.yaml     # Some tuning
    │       ├── kustomization.yaml
    │       └── proxy.yaml         # NO_PROXY, HTTP_PROXY, HTTPS_PROXY environment variables
    └── test
        └── suite_test.go          # Ginkgo based deployment test
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

Test Flow
---------

CI in this repository expects running deployment test using `neco-ops` instance. Test resources are in `test/` directory of each `appname`.
The typical test step is:

- Run [Ginkgo][] based deployment test.
    1. Load initialized state of the placemat snapshot by `pmctl snapshot load`.
    2. Login to `neco-ops` instance.
    3. Deploy Argo CD by `kubectl`.
    4. Initialize Argo CD client with `argocd login SERVER --name admin --password xxxxx`.
    5. Deploy Argo CD configuration `argocd-config` by:
        ```console
        argocd app create argocd-config -f https://github.com/cybozu-go/neco-apps --path argocd-config/overlays/gcp --dest-namespace=argocd ...
        ```
    6. Deploy `argocd-config` and other apps through Argo CD by `argocd app sync APPNAME`.
    7. Check some status.

How to debug CI result
----------------------

1. Login to `neco-ops` instance.

```console
gcloud compute --project=neco-test ssh cybozu@neco-ops
```

2. You can find cloned `neco-apps` repository in `$HOME/${CIRCLE_PROJECT_REPONAME}-${CIRCLE_BUILD_NUM}/go/src/github.com/cybozu-go/neco-apps`.
3. ssh to the boot server.
```console
cd ${CIRCLE_PROJECT_REPONAME}-${CIRCLE_BUILD_NUM}/go/src/github.com/cybozu-go/neco-apps/test
./dcssh boot-0
```

[Ginkgo]: https://github.com/onsi/ginkgo
