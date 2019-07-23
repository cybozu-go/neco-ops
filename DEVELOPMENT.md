Guide for developers
====================

This repository contains Kubernetes manifests that will be delivered to
Neco data centers automatically by [Argo CD][].

Adding a new application
------------------------

Determine the name of the new application, then make `base` directory for it:

```console
$ APP_NAME=foo
$ mkdir -p ${APP_NAME}/base
```

### Common manifests

Manifests that are common to all environments go to `base` directory.

A manifest can be incomplete as long as it will be complemented by
*overlays* as described later.

Note that manifests extension should be `.yaml` instead of `.yml`
because ArgoCD searches only `*.yaml` files.

```console
$ edit ${APP_NAME}/base/deployment.yaml
$ edit ${APP_NAME}/base/service.yaml
```

Then, create `kustomization.yaml` for [Kustomize][].

```console
$ cat <<EOF > ${APP_NAME}/base/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment.yaml
- service.yaml
EOF
```

### Environment-specific manifests

For each data center such as `gcp`, `stage0`, `tokyo0`, you may create
patch manifets as follows:

```console
$ ENV=stage0
$ mkdir -p ${APP_NAME}/overlays/${ENV}
$ vi ${APP_NAME}/overlays/${ENV}/deployment_replicas.yaml
```

Create `kustomization.yaml` to apply patches to base manifets.

```console
$ cat <<EOF > ${APP_NAME}/overlays/${ENV}/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
bases:
- ../../base
patches:
- deployment_replicas.yaml
EOF
```

### Verify manifests

```console
$ (cd test; make setup)
$ kustomize build ${APP_NAME}/base > /tmp/app.yaml
$ kustomize build ${APP_NAME}/overlays/${ENV} > /tmp/app_${ENV}.yaml
$ diff -u /tmp/app.yaml /tmp/app_${ENV}.yaml
```

Apply the generated manifest using `kubectl apply --dry-run -f FILE`
in the dctest environment.

### Add the application to Argo CD

Add following files under `argocd-config` directory:

```console
$ edit argocd-config/base/${APP_NAME}.yaml
$ edit argocd-config/overlays/${ENV}/${APP_NAME}.yaml
```

Examples are [argocd-config/base/metallb.yaml](argocd-config/base/metallb.yaml)
and [argocd-config/overlays/stage0/metallb.yaml](argocd-config/overlays/stage0/metallb.yaml).

Add the filenames to `kustomization.yaml` for ArgoCD.

```console
$ edit argocd-config/base/kustomization.yaml
$ edit argocd-config/overlays/${ENV}/kustomization.yaml
```

Testing
-------

Tests should be written using `ginkgo` and `gomega` as follows:

```console
$ edit ${APP_NAME}/test/suite_test.go
$ edit ${APP_NAME}/test/some_test.go
```

Add the new test to `TESTS` in `test/Makefile`.

Deployment
----------

### Deploy to staging environments

Merged changes to `master` branch will be applied automatically.

### Deploy to production environments

Argo CD in production environments refers to `release` branch.

CircleCI will create a pull request to update `release` branch
when a tag is pushed as follows:

```console
$ git checkout master
$ TAG=release-$(date +%Y.%m.%d)-1
$ git tag $TAG
$ git push origin $TAG
```

Then merge the pull request.

Trouble shooting
----------------

Make sure you are logging in Argo CD.

### Out of sync

When `argocd app list` shows `OutOfSync` status for an application as follows:

```console
$ argocd app list
NAME           CLUSTER                         NAMESPACE       PROJECT  STATUS     HEALTH       SYNCPOLICY  CONDITIONS
argocd-config  https://kubernetes.default.svc  argocd          default  Synced     Healthy      <none>      <none>
metallb        https://kubernetes.default.svc  metallb-system  default  Synced     Progressing  <none>      <none>
monitoring     https://kubernetes.default.svc  monitoring      default  OutOfSync  Missing      <none>      <none>
```

Try manual synchronization or investigate further details:

```console
$ argocd app sync monitoring
$ argocd app get monitoring
```

Or use `kubectl describe` for Kubernetes resources:

```console
$ kubectl describe -n monitoring deployment alertmanager
```

[Argo CD]: https://github.com/argoproj/argo-cd
[Kustomize]: https://github.com/kubernetes-sigs/kustomize
