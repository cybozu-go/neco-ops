```
mkdir /tmp/loki
cd /tmp/loki/
go get -u github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb
tk init
tk env add environments/loki --namespace=logging
// ここでバージョンを指定したい
jb install github.com/grafana/loki/production/ksonnet/loki@v2.2.0
// kubernetes バージョンを見ながら
jb install github.com/jsonnet-libs/k8s-alpha/1.19
echo "import 'github.com/jsonnet-libs/k8s-alpha/1.19/main.libsonnet'" > lib/k.libsonnet

cp ~/go/src/github.com/cybozu-go/neco-apps/logging/main.jsonnet /tmp/loki/environments/loki/main.jsonnet; rm -rf ~/go/src/github.com/cybozu-go/neco-apps/logging/base/loki/upstream/*; tk export ~/go/src/github.com/cybozu-go/neco-apps/logging/base/loki/upstream/ /tmp/loki/environments/loki/
```
