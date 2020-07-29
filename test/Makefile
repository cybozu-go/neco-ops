BOOT0 = 10.72.48.0
BOOT1 = 10.72.48.1
BOOT2 = 10.72.48.2
GINKGO = env GO111MODULE=on GOFLAGS=-mod=vendor $(GOPATH)/bin/ginkgo --failFast -v
SSH_PRIVKEY = $(NECO_DIR)/dctest/dctest_key
CIRCLE_BUILD_NUM ?= -$(USER)
TEST_ID := test$(CIRCLE_BUILD_NUM)
BASE_BRANCH = master
COMMIT_ID = $(shell git rev-parse --abbrev-ref HEAD)
KUSTOMIZATION_DIRS := $(shell find ../ -name "kustomization.yaml" -exec dirname {} \;)
SUDO = sudo
WGET=wget --retry-connrefused --no-verbose
NUM_DASHBOARD = $(shell KUSTOMIZE_ENABLE_ALPHA_COMMANDS=true kustomize config count ../monitoring/base/grafana-operator/dashboards | \
	grep GrafanaDashboard | cut -d' ' -f2)
export BOOT0 BOOT1 BOOT2 GINKGO SSH_PRIVKEY TEST_ID COMMIT_ID KUSTOMIZATION_DIRS BOOTSTRAP NUM_DASHBOARD

### for Go
GOFLAGS = -mod=vendor
export GOFLAGS

# Follow Argo CD installed kustomize version
# https://github.com/cybozu/neco-containers/blob/master/argocd/Dockerfile#L32
KUSTOMIZE_VERSION = 3.6.1
PROMTOOL_VERSION := 2.12.0
TELEPORT_VERSION := 4.2.10
KUBERNETES_VERSION = 1.17.4
ARGOCD_VERSION = 1.5.7

install.yaml: $(shell find ../argocd/base)
	kustomize build ../argocd/base/ > install.yaml

validation:
	./fetch-secret.sh
	env SSH_PRIVKEY= go test -v ./validation_test.go

test-alert-rules:
	promtool test rules alert_test/*.yaml

kustomize-check:
	./kustomize-check.sh

code-check:
	test -z "$$(gofmt -s -l . | grep -v -E '^(vendor|neco-apps)' | tee /dev/stderr)"
	test -z "$$(golint $$(go list ./... | grep -v /vendor/) | grep -v '/mtest/.*: should not use dot imports' | tee /dev/stderr)"
	test -z "$$(nilerr ./... 2>&1 | tee /dev/stderr)"
	test -z "$$(custom-checker -restrictpkg.packages=html/template,log ./... 2>&1 | tee /dev/stderr)"
	ineffassign .
	go vet ./...

test: code-check install.yaml test-alert-rules kustomize-check validation

dctest: install.yaml
	./test.sh

dctest-ceph: install.yaml
	CEPH=1 ./test.sh

dctest-reboot: install.yaml
	REBOOT=1 ./test.sh

dctest-upgrade:
	-git worktree remove /tmp/neco-apps
	git worktree add /tmp/neco-apps $(BASE_BRANCH)
	cp account.json /tmp/neco-apps/test
	cd /tmp/neco-apps/test; \
		if [ -f Makefile.dctest ]; then \
			$(MAKE) test-apps BOOTSTRAP=1 COMMIT_ID=$(BASE_BRANCH); \
		else \
			$(MAKE) dctest BOOTSTRAP=1 COMMIT_ID=$(BASE_BRANCH); \
		fi
	cp /tmp/neco-apps/test/argocd-password.txt ./
	UPGRADE=1 ./test.sh

setup:
	$(SUDO) apt-get update && $(SUDO) apt-get -y install wget
	curl -sSLf -O https://storage.googleapis.com/kubernetes-release/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl
	$(SUDO) mv kubectl /usr/local/bin/kubectl
	chmod +x /usr/local/bin/kubectl
	$(SUDO) bash -c "kubectl completion bash > /etc/bash_completion.d/kubectl"
	curl -sSLf https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_linux_amd64.tar.gz | tar zxf - -C /tmp
	$(SUDO) mv /tmp/kustomize /usr/local/bin/kustomize
	chmod +x /usr/local/bin/kustomize
	go install github.com/onsi/ginkgo/ginkgo
	curl -sSLf https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).linux-amd64.tar.gz | tar -xzvf - --strip-components=1 prometheus-$(PROMTOOL_VERSION).linux-amd64/promtool
	$(SUDO) mv promtool /usr/local/bin/promtool
	$(WGET) -O /tmp/teleport.tgz https://get.gravitational.com/teleport-v$(TELEPORT_VERSION)-linux-amd64-bin.tar.gz
	tar -xzvf /tmp/teleport.tgz --strip-component=1 teleport/tsh
	rm -f /tmp/teleport.tgz
	$(SUDO) mv tsh /usr/local/bin/tsh
	curl -sSLf -O https://github.com/argoproj/argo-cd/releases/download/v$(ARGOCD_VERSION)/argocd-linux-amd64
	$(SUDO) mv argocd-linux-amd64 /usr/local/bin/argocd
	chmod +x /usr/local/bin/argocd
	$(SUDO) bash -c "argocd completion bash > /etc/bash_completion.d/argocd"

clean:
	rm -f install.yaml

.PHONY:	validation test-alert-rules kustomize-check code-check test \
		dctest dctest-ceph dctest-reboot dctest-upgrade setup clean
