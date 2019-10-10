How to run tests
================

dctest
------

1. Prepare dctest environment using `github.com/cybozu-go/neco/dctest`
2. Set `NECO_DIR` environment variable to point the directory for `github.com/cybozu-go/neco`
3. Place `account.json` file for GCP Cloud DNS in this directory.
4. Push the current feature branch to GitHub.
5. Run following commands to setup Argo CD and run tests.

    ```console
    cd test
    make setup
    make test
    make -f Makefile.dctest dctest
    ```

kindtest
--------

1. Push the current feature branch to GitHub.
2. Run following commands to setup Argo CD and run tests.

    ```console
    cd test
    make setup
    make test
    make -f Makefile.kindtest start
    make -f Makefile.kindtest kindtest
    ```

`./account.json`
----------------

External DNS in Argo CD app `external-dns` requires Google Application Credentials in JSON file.
neco-apps test runs `kubectl create secrets .... --from-file=account.json` to register `Secret` for External DNS.
To run `external-dns` test, put your account.json of the Google Cloud service account which has a role `roles/dns.admin`.
See details of the role at https://cloud.google.com/iam/docs/understanding-roles#dns-roles

Using `argocd`
--------------

`argocd` is a command-line tool to manage Argo CD apps.

Following features are most useful:

- `argocd app list`: list apps and their statuses.
- `argocd app get NAME`: show detailed information of an app.
- `argocd app sync NAME`: immediately synchronize an app with Git repository.

Makefile
--------

You can run test for neco-apps on the running dctest.

- `make setup`: Install test required components.
- `make clean`: Delete generated files.
- `make test`: Run `gofmt` and other trivial tests.
- `make validation`: Run validation test of manifests.
- `make opa-test`: Run unit test of OPA Rego files.
- `make test-alert-rules`: Run unit test of Prometheus alerts.
- `make kustomize-check`: Check syntax of the Kubernetes manifests using `kustomize check`
