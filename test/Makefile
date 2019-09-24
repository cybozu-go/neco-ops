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
export BOOT0 BOOT1 BOOT2 GINKGO SSH_PRIVKEY TEST_ID COMMIT_ID KUSTOMIZATION_DIRS BOOTSTRAP

### for Go
GOFLAGS = -mod=vendor
export GOFLAGS

# Follow Argo CD installed kustomize version
# https://github.com/cybozu/neco-containers/blob/master/argocd/Dockerfile#L32
KUSTOMIZE_VERSION = 2.0.3
OPA_VERSION = 0.12.0
PROMTOOL_VERSION := 2.9.2
TELEPORT_VERSION := 4.0.2
KUBERNETES_VERSION = 1.15.3
KIND_VERSION = 0.5.1
ARGOCD_VERSION = 1.1.2
REGO_FILES = $(shell find .. -path ../vendor -prune -o -name '*.rego' -print)

install.yaml: $(shell find ../argocd/base)
	kustomize build ../argocd/base/ > install.yaml

validation:
	go test -v ./validation_test.go

opa-test:
	test -z "$$(opa fmt -l ../ | grep -v '^vendor' | tee /dev/stderr)"
	opa test -v $(REGO_FILES)

test-alert-rules:
	cd ../monitoring/base/prometheus && promtool test rules test_rules.yaml

kustomize-check:
	./kustomize-check.sh

test: install.yaml validation opa-test test-alert-rules kustomize-check
	test -z "$$(gofmt -s -l . | grep -v -E '^(vendor|neco-apps)' | tee /dev/stderr)"
	test -z "$$(golint $$(go list ./... | grep -v /vendor/) | grep -v '/mtest/.*: should not use dot imports' | tee /dev/stderr)"
	test -z "$$(nilerr ./... 2>&1 | tee /dev/stderr)"
	test -z "$$(restrictpkg -packages=html/template,log ./... 2>&1 | tee /dev/stderr)"
	ineffassign .
	go vet ./...

test-apps:
	$(MAKE) -f Makefile.dctest dctest

setup:
	curl -sSLf -O https://storage.googleapis.com/kubernetes-release/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl
	$(SUDO) mv kubectl /usr/local/bin/kubectl
	chmod +x /usr/local/bin/kubectl
	curl -sSLf -O https://github.com/kubernetes-sigs/kustomize/releases/download/v$(KUSTOMIZE_VERSION)/kustomize_$(KUSTOMIZE_VERSION)_linux_amd64
	$(SUDO) mv kustomize_$(KUSTOMIZE_VERSION)_linux_amd64 /usr/local/bin/kustomize
	chmod +x /usr/local/bin/kustomize
	curl -sSLf -O https://github.com/open-policy-agent/opa/releases/download/v$(OPA_VERSION)/opa_linux_amd64
	$(SUDO) mv opa_linux_amd64 /usr/local/bin/opa
	chmod +x /usr/local/bin/opa
	go install github.com/onsi/ginkgo/ginkgo
	curl -sSLf https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).linux-amd64.tar.gz | tar -xzvf - --strip-components=1 prometheus-$(PROMTOOL_VERSION).linux-amd64/promtool
	$(SUDO) mv promtool /usr/local/bin/promtool
	curl -sSLf https://get.gravitational.com/teleport-v$(TELEPORT_VERSION)-linux-amd64-bin.tar.gz | tar -xzvf - --strip-component=1 teleport/tsh
	$(SUDO) mv tsh /usr/local/bin/tsh
	cd /tmp; env GOFLAGS= GO111MODULE=on go get sigs.k8s.io/kind@v$(KIND_VERSION)
	curl -sSLf -O https://github.com/argoproj/argo-cd/releases/download/v$(ARGOCD_VERSION)/argocd-linux-amd64
	$(SUDO) mv argocd-linux-amd64 /usr/local/bin/argocd
	chmod +x /usr/local/bin/argocd
	env GOFLAGS= GO111MODULE=off go get github.com/gostaticanalysis/nilerr/cmd/nilerr
	env GOFLAGS= GO111MODULE=off go get github.com/cybozu/neco-containers/golang/restrictpkg/cmd/restrictpkg

clean:
	rm -f install.yaml

.PHONY:	validation opa-test test-alert-rules kustomize-check test test-apps setup clean
