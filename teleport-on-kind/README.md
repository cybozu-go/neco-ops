Teleport on kind
================

* setup kind

```console
$ GO111MODULE="on" go get sigs.k8s.io/kind@v0.4.0
```

* create k8s cluster

```console
$ kind create cluster --image kindest/node:v1.14.2 --name=multi --config cluster.yaml
$ export KUBECONFIG="$(kind get kubeconfig-path --name="multi")"
```

* setup helm

```console
$ sudo snap install helm --classic
$ kubectl create -f rbac-config.yaml
$ helm init --service-account tiller --history-max 200
```

* generate certificates

```console
$ ./create-internal-pki.sh
$ cd pki
$ kubectl create secret tls tls-web --cert=proxy-server.pem --key=proxy-server-key.pem
$ kubectl create configmap ca-certs --from-file=ca.pem
```

* setup teleport

```console
$ git clone https://github.com/gravitational/teleport.git
$ helm install -f values.yaml $GOPATH/src/github.com/gravitational/teleport/examples/chart/teleport/
```

* port forward

```console
$ kubectl port-forward teleport-xxxx 3080:3080 3025:3025 3023:3023 3026:3026 3024:3024
```

* add user

```console
$ kubectl exec -it teleport-xxxx bash
$ tctl users add xxxxx

Signup token has been created and is valid for 1 hours. Share this URL with the user:
https://teleport.example.com:3080/web/newuser/xxxxxxxxxxxxxxxxxxxxx
```

You can log in to teleport from above URL.
