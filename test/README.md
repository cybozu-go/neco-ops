How to run tests
================

1. Prepare dctest environment using `github.com/cybozu-go/neco/dctest`
2. Make snapshot of placemat VMs by `make save`
3. Push the current feature branch to GitHub.
4. Run following commands to setup Argo CD.

    ```console
    cd test
    make setup
    make test
    ```

5. Setup and run tests for apps by `make test-metallb`, `make test-monitoring`, ...  
   If you want to deploy all apps, run `make test-all`.

Running `make test` initializes VMs at the point of the snapshot.
Do it carefully!

Using `argocd`
--------------

`argocd` is a command-line tool to manage Argo CD apps.

Following features are most useful:

* `argocd app list`: list apps and their statuses.
* `argocd app get NAME`: show detailed information of an app.
* `argocd app sync NAME`: immediately synchronize an app with Git repository.


Makefile
--------

You can run test for neco-apps on the running dctest.

* `make setup`

    Install test required components.

* `make clean`

    Delete generated files.

* `make kustomize-check`

     Check syntax of the Kubernetes manifests using `kustomize check`

* `make test`

    Deploy Argo CD as test. It is required other test targets which starts with `test-`.

* `make test-APPNAME`

    Deploy APPNAME as test.

* `make test-all`

    Deploy all applications as test.

Options
-------

### `./account.json` file

External DNS in Argo CD app `external-dns` requires Google Application Credentials in JSON file.
neco-apps test runs `kubectl create secrets .... --from-file=account.json` to register `Secret` for External DNS.
To run `external-dns` test, put your account.json of the Google Cloud service account which has a role `roles/dns.admin`.
See details of the role at https://cloud.google.com/iam/docs/understanding-roles#dns-roles
