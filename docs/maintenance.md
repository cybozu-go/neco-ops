How to maintain neco-apps
=========================

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
$ curl -sLf -o  cert-manager/base/upstream/cert-manager.yaml https://github.com/jetstack/cert-manager/releases/download/vX.Y.Z/cert-manager.yaml
$ vi cert-manager/base/upstream/cert-manager.yaml
  (Remove Namespace resources)
```

## elastic (ECK)

To check diffs between versions, download and compare manifests as follows:

```console
$ wget https://download.elastic.co/downloads/eck/X.Y.Z/all-in-one.yaml
$ vi elastic/base/upstream/all-in-one.yaml
  (Remove Namespace resources)
```

## external-dns

Read the following document and fix manifests as necessary.

https://github.com/kubernetes-sigs/external-dns/blob/vX.Y.Z/docs/tutorials/coredns.md

## ingress (Contour & Envoy)

Check diffs of projectcontour/contour files as follows:

```console
$ git clone https://github.com/projectcontour/contour
$ cd contour
$ git diff vA.B.C...vX.Y.Z examples/contour
```

Then, import YAML manifests as follows:

```console
$ git checkout vX.Y.Z
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

## monitoring

### prometheus, alertmanager, grafana

There is no official kubernetes manifests for prometheus, alertmanager, and grafana.
So, check changes in release notes on github and take necessary actions.

### machines-endpoints

Update version following [this link](https://github.com/cybozu/neco-containers/blob/master/machines-endpoints/TAG)

### kube-state-metrics

Check [examples/standard](https://github.com/kubernetes/kube-state-metrics/tree/master/examples/standard)

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

## rook

*Do not upgrade Rook and Ceph at the same time!*

Read [this document](https://github.com/rook/rook/blob/master/Documentation/ceph-upgrade.md) before.

Get upstream helm chart:

```console
$ cd $GOPATH/src/github.com/rook
$ git clone https://github.com/rook/rook
$ cd rook
$ git remote add fork git@github.com:cybozu-go/rook.git
$ git fetch fork
$ git checkout neco-release
$ ls $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
$ rm -rf $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
$ cp -a cluster/charts/rook-ceph $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base/upstream/chart
```

Download Helm used in Rook. Follow `HELM_VERSION` in the upstream configuration.

```console
# Check the Helm version, in rook repo directory downloaded above
$ cat $GOPATH/src/github.com/rook/rook/build/makelib/helm.mk | grep ^HELM_VERSION
$ HELM_VERSION=X.Y.Z
$ curl -sSLf https://get.helm.sh/helm-v$HELM_VERSION-linux-amd64.tar.gz | sudo tar -C /usr/local/bin linux-amd64/helm --strip-components 1 -xzf -
```

Update rook/base/values*.yaml if necessary.

Regenerate base resource yaml  
note: check number of yaml files.

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps/rook/base
$ for i in clusterrole psp resources; do
    helm template upstream/chart -f values.yaml -x templates/${i}.yaml > common/${i}.yaml
  done
$ for t in hdd ssd; do
    for i in deployment role rolebinding serviceaccount; do
      helm template upstream/chart -f values.yaml -f values-${t}.yaml -x templates/${i}.yaml --namespace ceph-${t} > ceph-${t}/${i}.yaml
    done
    helm template upstream/chart -f values.yaml -f values-${t}.yaml -x templates/clusterrolebinding.yaml --namespace ceph-${t} > ceph-${t}/clusterrolebinding/clusterrolebinding.yaml
  done
```

Then check the diffs by `git diff`.

TODO:  
After https://github.com/rook/rook/pull/5240 is merged, we have to revise above mentioned process.

Update manifest for Ceph toolbox.
Assume `cybozu-go/rook` is updated in the above procedure.

```console
$ cd $GOPATH/src/github.com/cybozu-go/neco-apps/
$ cp $GOPATH/src/github.com/cybozu-go/rook/cluster/examples/kubernetes/ceph/toolbox.yaml rook/base/upstream/
```

## ceph

*Do not upgrade Rook and Ceph at the same time!*

Read [this document](https://github.com/rook/rook/blob/master/Documentation/ceph-upgrade.md) before.

Update `spec.cephVersion.image` field in CephCluster CR.

- rook/base/ceph-hdd/cluster.yaml
- rook/base/ceph-ssd/cluster.yaml

## teleport

There is no official kubernetes manifests actively maintained for teleport.
So, check changes in [CHANGELOG.md](https://github.com/gravitational/teleport/blob/master/CHANGELOG.md) on github.

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
