[![CircleCI](https://circleci.com/gh/cybozu-go/neco-apps.svg?style=svg)](https://circleci.com/gh/cybozu-go/neco-apps)


Neco Apps
=========

A collection of Kubernetes applications to make a vanilla Kubernetes useful for soft multi-tenancy environments.

This repository is a part of [Neco project](https://github.com/cybozu-go/neco/).
The application manifests are organized for [Kustomize](https://github.com/kubernetes-sigs/kustomize) and [Argo CD](https://argoproj.github.io/argo-cd/).

Environments
------------

Currently, the following environments are defined:

- `gcp`: A fully virtualized data center built with [neco/dctest](https://github.com/cybozu-go/neco/tree/main/dctest).
- `stage0`: A real data center of Neco project for staging usage.
- `tokyo0`, `osaka0`: Real data centers of Neco project for production usage.

Development
-----------

- CI/CD: read [docs/cicd.md](docs/cicd.md).
- Manifests: read [docs/manifests.md](docs/manifests.md).
- Deploy: read [docs/deploy.md](docs/deploy.md).
- Tests: read [test/README.md](test/README.md).
