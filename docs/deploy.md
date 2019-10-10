How to deploy and release changes as GitOps
===========================================

Staging cluster
---------------

### Setup Argo CD

*NOTE: Skip this step if Argo CD is already deployed*

1. Install Argo CD command into your client.
2. Deploy Argo CD like `../test/install.yaml` into your Kubernetes cluster.
3. Login to the exposed `argocd-server` service.
4. Create `argocd-config` application as follows:
    ```console
    $ argocd app create argocd-config \
      --repo https://github.com/cybozu-go/neco-apps.git \
      --path argocd-config/overlays/$(cat /etc/neco/cluster) \
      --dest-namespace argocd \
      --dest-server https://kubernetes.default.svc \
      --sync-policy automated \
      --auto-prune \
      --revision stage
    ```
5. Argo CD in the staging cluster watches changes of the **stage HEAD** branch.

### Apply changes

1. Developer makes changes, commits, and PR which merges to `master` branch.
2. Test by CI, Reviewer review, and accept if LGTM.
3. Reviewer merges PR.
4. Daily CI tests `master` branch and merges it to `stage` branch, then trigger similar job to secret repository.
5. In the secret repository, tests `master` branch and merges it to `stage` branch as same as step 4.
6. Argo CD synchronizes latest stage HEAD.

Production cluster
------------------

### Setup Argo CD

*NOTE: Skip this step if Argo CD is already deployed*

1. Setup Argo CD is as same as Staging cluster.
2. Create `argocd-config` application as follows:
    ```console
    $ argocd app create argocd-config \
      --repo https://github.com/cybozu-go/neco-apps.git \
      --path argocd-config/overlays/$(cat /etc/neco/cluster) \
      --dest-namespace argocd \
      --dest-server https://kubernetes.default.svc \
      --sync-policy automated \
      --auto-prune \
      --revision release
    ```
3. Argo CD in the production cluster watches changes of the **release HEAD** branch.

### Apply changes

1. CircleCI has already prepared pull requests to merge changes in `stage` branch to `release` branch.
    Review the latest pull request and confirm the stability of `staging0` data center.
    If it looks good, merge it.  If not, close it.
    Old pull requests should be closed.
2. Argo CD synchronizes latest release HEAD.

Glossary
--------

- tag: `release-YYYY.MM.DD-UNIQUE_ID`

    It is a candidate release version which is passed CI and the evaluation in the staging cluster.

- [cybozu-neco][] 🐈

    A bot GitHub Account for handling some CI jobs using GitHub.

- Secret repository

    GitOps for secret repository for internal use only.

[cybozu-neco]: https://github.com/cybozu-neco
