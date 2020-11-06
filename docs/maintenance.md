How to maintain neco-apps
=========================

- [argocd](#argocd)
- [cert-manager](#cert-manager)
- [elastic (ECK)](#elastic-eck)
- [external-dns](#external-dns)
- [ingress (Contour & Envoy)](#ingress-contour--envoy)
- [metallb](#metallb)
- [metrics-server](#metrics-server)
- [monitoring](#monitoring)
  - [prometheus, alertmanager, pushgateway](#prometheus-alertmanager-pushgateway)
  - [machines-endpoints](#machines-endpoints)
  - [kube-state-metrics](#kube-state-metrics)
  - [grafana-operator](#grafana-operator)
- [neco-admission](#neco-admission)
- [network-policy (Calico)](#network-policy-calico)
- [pvc-autoresizer](#pvc-autoresizer)
- [rook](#rook)
  - [ceph](#ceph)
- [teleport](#teleport)
- [topolvm](#topolvm)
- [moco](#moco)

## argocd

1. Check [releases](https://github.com/argoproj/argo-cd/releases) for changes.

1. Check [upgrading overview](https://github.com/argoproj/argo-cd/blob/master/docs/operator-manual/upgrading/overview.md) when upgrading major or minor version.

1. Download the upstream manifest as follows:
   ```console
   $ curl -sLf -o argocd/base/upstream/install.yaml https://raw.githubusercontent.com/argoproj/argo-cd/vX.Y.Z/manifests/install.yaml
   ```
   Then check the diffs by `git diff`.

## cert-manager

Check [the upgrading section](https://cert-manager.io/docs/installation/upgrading/) in the official website.

Download manifests and remove `Namespace` resource from it as follows:

```console
$ curl -sLf -o cert-manager/base/upstream/cert-manager.yaml https://github.com/jetstack/cert-manager/releases/download/vX.Y.Z/cert-manager.yaml
$ vi cert-manager/base/upstream/cert-manager.yaml
  (Remove Namespace resources)
```

## elastic (ECK)

Check the [Upgrade ECK](https://www.elastic.co/guide/en/cloud-on-k8s/current/k8s-upgrading-eck.html) in the official website.

Download manifests and remove `Namespace` resource from it as follows:

```console
$ curl -sLf -o elastic/base/upstream/all-in-one.yaml https://download.elastic.co/downloads/eck/X.Y.Z/all-in-one.yaml
$ vi elastic/base/upstream/all-in-one.yaml
  (Remove Namespace resources)
```

## external-dns

Read the following document and fix manifests as necessary.

https://github.com/kubernetes-sigs/external-dns/blob/vX.Y.Z/docs/tutorials/coredns.md

Download CRD manifest as follows:

```console
$ curl -sLf -o external-dns/base/common.yaml https://github.com/kubernetes-sigs/external-dns/blob/vX.Y.Z/docs/contributing/crd-source/crd-manifest.yaml
```
Then check the diffs by `git diff`.

## ingress (Contour & Envoy)

Check the [upgrading guide](https://projectcontour.io/resources/upgrading/) in the official website.

Check diffs of projectcontour/contour files as follows:

```console
$ git clone https://github.com/projectcontour/contour
$ cd contour
$ git diff vA.B.C...vX.Y.Z examples/contour
```

Then, import YAML manifests as follows:

```console
$ git checkout vX.Y.Z
$ rm $GOPATH/src/github.com/cybozu-go/neco-apps/ingress/base/contour/*
$ cp examples/contour/*.yaml $GOPATH/src/github.com/cybozu-go/neco-apps/ingress/base/contour/
```

Note that:
- We do not use contour's certificate issuance feature, but use cert-manager to issue certificates required for gRPC.
- We change Envoy manifest from DaemonSet to Deployment.
- Not all manifests inherit the upstream. Please check `kustomization.yaml` which manifest inherits or not.
  - If the manifest in the upstream is usable as is, use it from `ingress/base/kustomization.yaml`.
  - If the manifest needs modification:
    - If the manifest is for a cluster-wide resource, put a modified version in the `common` directory.
    - If the manifest is for a namespaced resource, put a template in the `template` directory and apply patches.

## metallb

Check [releases](https://github.com/metallb/metallb/releases)

Download manifests and remove `Namespace` resource from it as follows:

```console
$ git clone https://github.com/metallb/metallb
$ cd metallb
$ git checkout vX.Y.Z
$ cp manifests/*.yaml $GOPATH/src/github.com/cybozu-go/neco-apps/metallb/base/upstream
```

## metrics-server

Check [releases](https://github.com/kubernetes-sigs/metrics-server/releases)

Download the upstream manifest as follows:

```console
$ git clone https://github.com/kubernetes-sigs/metrics-server
$ cd metrics-server
$ git checkout vX.Y.Z
$ cp deploy/1.8+/*.yaml $GOPATH/src/github.com/cybozu-go/neco-apps/metrics-server/base/upstream
```

Note: The name of `deploy` directory will be changed.

## monitoring

### prometheus, alertmanager, pushgateway

There is no official kubernetes manifests for prometheus, alertmanager, and grafana.
So, check changes in release notes on github and helm charts like bellow.

```
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm search repo -l prometheus-community
helm template prom prometheus-community/prometheus --version=11.5.0 > prom-2.18.1.yaml
helm template prom prometheus-community/prometheus --version=11.16.7 > prom-2.21.0.yaml
diff prom-2.18.1.yaml prom-2.21.0.yaml
```

### machines-endpoints

Update version following [this link](https://github.com/cybozu/neco-containers/blob/master/machines-endpoints/TAG)

### kube-state-metrics

Check [examples/standard](https://github.com/kubernetes/kube-state-metrics/tree/master/examples/standard)

### grafana-operator

Check [releases](https://github.com/integr8ly/grafana-operator/releases)

Download the upstream manifest as follows:

```console
$ git clone https://github.com/integr8ly/grafana-operator
$ cd grafana-operator
$ git checkout vX.Y.Z
$ cp -r deploy/* $GOPATH/src/github.com/cybozu-go/neco-apps/monitoring/base/grafana-operator/upstream
```

## neco-admission

Update version following [this link](https://github.com/cybozu/neco-containers/blob/master/admission/TAG)

## network-policy (Calico)

Check [the release notes](https://docs.projectcalico.org/release-notes/).

Download the upstream manifest as follows:

```console
$ curl -sLf -o network-policy/base/calico/upstream/calico-policy-only.yaml https://docs.projectcalico.org/vX.Y/manifests/calico-policy-only.yaml
```

Remove the resources related to `calico-kube-controllers` from `calico-policy-only.yaml` because we do not need to use `calico/kube-controllers`.
See: [Kubernetes controllers configuration](https://docs.projectcalico.org/reference/resources/kubecontrollersconfig)

## pvc-autoresizer

Check [the CHANGELOG](https://github.com/topolvm/pvc-autoresizer/blob/master/CHANGELOG.md).

Download the upstream tar ball from [releases](https://github.com/topolvm/pvc-autoresizer/releases/latest) and generate upstream manifests as follows:

```console
$ kustomize build ./config/default > /path/to/pvc-autoresizer/base/upstream.yaml
```

## rook

*Do not upgrade Rook and Ceph at the same time!*

Read [this document](https://github.com/rook/rook/blob/master/Documentation/ceph-upgrade.md) before. Note that you should choose the appropriate release version.

Get upstream helm chart:

```console
$ cd $GOPATH/src/github.com/rook
$ git clone https://github.com/rook/rook
$ cd rook
$ ROOK_VERSION=X.Y.Z
$ git checkout v$ROOK_VERSION
$ ls $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
$ rm -rf $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
$ cp -a cluster/charts/rook-ceph $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
```

Download Helm used in Rook. Follow `HELM_VERSION` in the upstream configuration.

```console
# Check the Helm version, in rook repo directory downloaded above
$ cat $GOPATH/src/github.com/rook/rook/build/makelib/helm.mk | grep ^HELM_VERSION
$ HELM_VERSION=X.Y.Z
$ mkdir -p $GOPATH/src/github.com/cybozu-go/neco-apps/rook/bin
$ curl -sSLf https://get.helm.sh/helm-v$HELM_VERSION-linux-amd64.tar.gz | tar -C $GOPATH/src/github.com/cybozu-go/neco-apps/rook/bin linux-amd64/helm --strip-components 1 -xzf -
```

Update rook/base/values*.yaml if necessary.

Regenerate base resource yaml.  
Note: Check the number of yaml files.

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base
$ for i in clusterrole psp resources; do
    ../bin/helm template upstream/chart -f values.yaml -x templates/${i}.yaml > common/${i}.yaml
  done
$ for t in hdd ssd; do
    for i in deployment role rolebinding serviceaccount; do
      ../bin/helm template upstream/chart -f values.yaml -f values-${t}.yaml -x templates/${i}.yaml --namespace ceph-${t} > ceph-${t}/${i}.yaml
    done
    ../bin/helm template upstream/chart -f values.yaml -f values-${t}.yaml -x templates/clusterrolebinding.yaml --namespace ceph-${t} > ceph-${t}/clusterrolebinding/clusterrolebinding.yaml
  done
```

Then check the diffs by `git diff`.

TODO:  
After https://github.com/rook/rook/pull/5573 is merged, we have to revise the above-mentioned process.

Update manifest for Ceph toolbox.
Assume `rook/rook` is updated in the above procedure.

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps/
$ cp $GOPATH/src/github.com/rook/rook/cluster/examples/kubernetes/ceph/toolbox.yaml rook/base/upstream/
```

Update rook/**/kustomization.yaml if necessary.

### ceph

*Do not upgrade Rook and Ceph at the same time!*

Read [this document](https://github.com/rook/rook/blob/master/Documentation/ceph-upgrade.md) first.

Update `spec.cephVersion.image` field in CephCluster CR.

- rook/base/ceph-hdd/cluster.yaml
- rook/base/ceph-ssd/cluster.yaml

## teleport

There is no official kubernetes manifests actively maintained for teleport.
So, check changes in [CHANGELOG.md](https://github.com/gravitational/teleport/blob/master/CHANGELOG.md) on github,
and [Helm chart](https://github.com/gravitational/teleport/tree/master/examples/chart/teleport).

## topolvm

Check [releases](https://github.com/cybozu-go/topolvm/releases) for changes.

Download the upstream manifest as follows:

```console
$ cd $GOPATH/src/github.com/topolvm
$ git clone https://github.com/topolvm/topolvm
$ cd topolvm
$ git checkout vX.Y.Z
$ cp -r deploy/manifests/* $GOPATH/src/github.com/cybozu-go/neco-apps/topolvm/base/upstream
```

Update `images.newTag` in `kustomization.yaml`.


## moco

Check [releases](https://github.com/cybozu-go/moco/releases) for changes.

Download the upstream manifest as follows:

```console
$ cd $GOPATH/src/github.com/moco
$ git clone https://github.com/moco/moco
$ cd moco
$ git checkout vX.Y.Z
$ cp -r config/* $GOPATH/src/github.com/cybozu-go/neco-apps/moco/base/upstream
```

Update `images.newTag` in `kustomization.yaml`.
