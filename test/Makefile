CEPH ?= 0
OVERLAY ?= gcp
BOOT0 = 10.72.48.0
BOOT1 = 10.72.48.1
BOOT2 = 10.72.48.2
GINKGO = env GO111MODULE=on GOFLAGS=-mod=vendor $(GOPATH)/bin/ginkgo --failFast -v
SSH_PRIVKEY = $(NECO_DIR)/dctest/dctest_key
CIRCLE_BUILD_NUM ?= -$(USER)
TEST_ID := test$(CIRCLE_BUILD_NUM)
BASE_BRANCH = master
COMMIT_ID = $(shell git rev-parse --abbrev-ref HEAD)
SUDO = sudo
WGET=wget --retry-connrefused --no-verbose
NUM_DASHBOARD = $(shell KUSTOMIZE_ENABLE_ALPHA_COMMANDS=true ./bin/kustomize cfg count ../monitoring/base/grafana-operator/dashboards | \
	grep GrafanaDashboard | cut -d' ' -f2)
export BOOT0 BOOT1 BOOT2 GINKGO SSH_PRIVKEY TEST_ID COMMIT_ID BOOTSTRAP NUM_DASHBOARD

### for Go
GOFLAGS = -mod=vendor
export GOFLAGS

# Follow Argo CD installed kustomize version
# https://github.com/cybozu/neco-containers/blob/master/argocd/Dockerfile#L32
KUSTOMIZE_VERSION = 3.7.0
PROMTOOL_VERSION := 2.21.0
TELEPORT_VERSION := 4.3.7
KUBERNETES_VERSION = 1.18.8

# Cache
DOWNLOAD_DIR := download
KUSTOMIZE_DLPATH := $(DOWNLOAD_DIR)/kustomize-v$(KUSTOMIZE_VERSION).tar.gz
PROMTOOL_DLPATH := $(DOWNLOAD_DIR)/prometheus-v$(PROMTOOL_VERSION).tar.gz
TELEPORT_DLPATH := $(DOWNLOAD_DIR)/teleport-v$(TELEPORT_VERSION).tar.gz

BINDIR := $(abspath $(CURDIR)/bin)
KUBECTL := $(BINDIR)/kubectl
KUSTOMIZE := $(BINDIR)/kustomize
PROMTOOL := $(BINDIR)/promtool
TSH := $(BINDIR)/tsh

install.yaml: $(shell find ../argocd/base)
	$(KUSTOMIZE) build ../argocd/base/ > install.yaml

validation:
	./fetch-secret.sh
	env SSH_PRIVKEY= go test -v ./validation_test.go

test-alert-rules:
	$(PROMTOOL) test rules alert_test/*.yaml

code-check: test-tools
	test -z "$$(gofmt -s -l . | grep -v -E '^(vendor|neco-apps)' | tee /dev/stderr)"
	staticcheck ./...
	nilerr ./...
	test -z "$$(custom-checker -restrictpkg.packages=html/template,log ./... 2>&1 | tee /dev/stderr)"
	go vet ./...

test: code-check install.yaml test-alert-rules validation

dctest: install.yaml
	PATH=$(BINDIR):$$PATH CEPH=$(CEPH) OVERLAY=$(OVERLAY) ./test.sh

dctest-reboot: install.yaml
	PATH=$(BINDIR):$$PATH CEPH=$(CEPH) OVERLAY=$(OVERLAY) REBOOT=1 ./test.sh

dctest-upgrade:
	-git worktree remove /tmp/neco-apps
	git worktree add /tmp/neco-apps $(BASE_BRANCH)
	cp account.json /tmp/neco-apps/test
	# workaround for fixing BINDIR evaluation
	# please replace the below line after this code is released
	# $(MAKE) -C /tmp/neco-apps/test setup dctest BOOTSTRAP=1 COMMIT_ID=$(BASE_BRANCH)
	cd /tmp/neco-apps/test; $(MAKE) setup dctest BOOTSTRAP=1 COMMIT_ID=$(BASE_BRANCH)
	cp /tmp/neco-apps/test/argocd-password.txt ./
	PATH=$(BINDIR):$$PATH CEPH=$(CEPH) OVERLAY=$(OVERLAY) UPGRADE=1 ./test.sh

$(KUBECTL):
	mkdir -p $(BINDIR)
	$(MAKE) setup-download
	$(WGET) -O $@ https://storage.googleapis.com/kubernetes-release/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl
	chmod +x $@

setup-download:
	if [ -z "$$(which wget)" ]; then \
		$(SUDO) apt-get update && $(SUDO) apt-get -y install wget; \
	fi
	mkdir -p $(DOWNLOAD_DIR)

$(KUSTOMIZE):
	mkdir -p $(BINDIR)
	$(MAKE) setup-download
	$(WGET) -O $(KUSTOMIZE_DLPATH) https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz
	tar zxf $(KUSTOMIZE_DLPATH) -C $(BINDIR)
	chmod +x $@

$(PROMTOOL):
	mkdir -p $(BINDIR)
	$(MAKE) setup-download
	$(WGET) -O $(PROMTOOL_DLPATH) https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).linux-amd64.tar.gz
	tar zxf $(PROMTOOL_DLPATH) -C $(BINDIR) --strip-components=1 prometheus-$(PROMTOOL_VERSION).linux-amd64/promtool

$(TSH):
	mkdir -p $(BINDIR)
	$(MAKE) setup-download
	$(WGET) -O $(TELEPORT_DLPATH) https://get.gravitational.com/teleport-v$(TELEPORT_VERSION)-linux-amd64-bin.tar.gz
	tar zxf $(TELEPORT_DLPATH) -C $(BINDIR) --strip-component=1 teleport/tsh

setup: $(KUBECTL) $(KUSTOMIZE) $(PROMTOOL) $(TSH)
	go install github.com/onsi/ginkgo/ginkgo

.PHONY: test-tools
test-tools: staticcheck nilerr

.PHONY: staticcheck
staticcheck:
	if ! which staticcheck >/dev/null; then \
		cd /tmp; env GOFLAGS= GO111MODULE=on go get honnef.co/go/tools/cmd/staticcheck; \
	fi

.PHONY: nilerr
nilerr:
	if ! which nilerr >/dev/null; then \
		cd /tmp; env GOFLAGS= GO111MODULE=on go get github.com/gostaticanalysis/nilerr/cmd/nilerr; \
	fi

clean:
	rm -rf $(DOWNLOAD_DIR)
	rm -f install.yaml
	rm -rf $(BINDIR)

.PHONY:	validation test-alert-rules code-check test \
		dctest dctest-ceph dctest-reboot dctest-upgrade setup-download setup clean
