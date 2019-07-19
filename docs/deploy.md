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
      --auto-prune
    ```
5. Argo CD in the staging cluster watches changes of the **master HEAD** branch.

### Apply changes

1. Developer makes changes, commits, and PR which merges to `master` branch.
2. Test by CI, Reviewer review, and accept if LGTM.
3. **Reviewer merges PR, and Argo CD synchronizes latest master HEAD.**

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

1. Deployment team confirms all changes the since last commit of `origin/release` branch are stable according to the CI result and the staging cluster deployment by `origin/master`.
2. Developer adds a git tag `release-YYYY.MM.DD-UNIQUE_ID` with `master HEAD` branch, and push the tag.  
    **TODO: This operation might be automated by the simple script**
    ```console
    $ git checkout master
    $ git pull
    $ git tag release-$(date +%Y.%m.%d)-UNIQUE_ID
    $ git push origin --tags
    ```
3. CI creates a new branch using its tag, then create a new PR which merges to `release` branch.
4. Reviewer reviews it, and accept if LGTM.
5. **Reviewer merges PR, and Argo CD synchronizes latest release HEAD.**

Backup cluster
--------------

**TBD**

Glossary
--------

- tag: `release-YYYY.MM.DD-UNIQUE_ID`

    It is a candidate release version which is passed CI and the evaluation in the staging cluster.

- [cybozu-neco][] 🐈

    A bot GitHub Account for handling some CI jobs using GitHub.

[cybozu-neco]: https://github.com/cybozu-neco
