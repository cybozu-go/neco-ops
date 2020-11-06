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
KUSTOMIZATION_DIRS := $(shell find ../ -name "kustomization.yaml" -exec dirname {} \;)
SUDO = sudo
WGET=wget --retry-connrefused --no-verbose
NUM_DASHBOARD = $(shell KUSTOMIZE_ENABLE_ALPHA_COMMANDS=true ./bin/kustomize cfg count ../monitoring/base/grafana-operator/dashboards | \
	grep GrafanaDashboard | cut -d' ' -f2)
export BOOT0 BOOT1 BOOT2 GINKGO SSH_PRIVKEY TEST_ID COMMIT_ID KUSTOMIZATION_DIRS BOOTSTRAP NUM_DASHBOARD

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

BINDIR := $(abspath $(PWD)/bin)
KUSTOMIZE := $(BINDIR)/kustomize
PROMTOOL := $(BINDIR)/promtool

install.yaml: $(shell find ../argocd/base)
	$(KUSTOMIZE) build ../argocd/base/ > install.yaml

validation:
	./fetch-secret.sh
	env SSH_PRIVKEY= go test -v ./validation_test.go

test-alert-rules:
	$(PROMTOOL) test rules alert_test/*.yaml

kustomize-check:
	./kustomize-check.sh

code-check: test-tools
	test -z "$$(gofmt -s -l . | grep -v -E '^(vendor|neco-apps)' | tee /dev/stderr)"
	staticcheck ./...
	nilerr ./...
	test -z "$$(custom-checker -restrictpkg.packages=html/template,log ./... 2>&1 | tee /dev/stderr)"
	go vet ./...

test: code-check install.yaml test-alert-rules kustomize-check validation

dctest: install.yaml
	CEPH=$(CEPH) OVERLAY=$(OVERLAY) ./test.sh

dctest-reboot: install.yaml
	CEPH=$(CEPH) OVERLAY=$(OVERLAY) REBOOT=1 ./test.sh

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
	CEPH=$(CEPH) OVERLAY=$(OVERLAY) UPGRADE=1 ./test.sh

setup-download:
	if [ -z "$$(which wget)" ]; then \
		$(SUDO) apt-get update && $(SUDO) apt-get -y install wget; \
	fi
	mkdir -p $(DOWNLOAD_DIR)

$(KUSTOMIZE_DLPATH):
	$(MAKE) setup-download
	$(WGET) -O $(KUSTOMIZE_DLPATH) https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz

$(PROMTOOL_DLPATH):
	$(MAKE) setup-download
	$(WGET) -O $(PROMTOOL_DLPATH) https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).linux-amd64.tar.gz

$(TELEPORT_DLPATH):
	$(MAKE) setup-download
	$(WGET) -O $(TELEPORT_DLPATH) https://get.gravitational.com/teleport-v$(TELEPORT_VERSION)-linux-amd64-bin.tar.gz

setup: $(KUSTOMIZE_DLPATH) $(PROMTOOL_DLPATH) $(TELEPORT_DLPATH)
	mkdir -p $(BINDIR)
	tar zxf $(KUSTOMIZE_DLPATH) -C /tmp
	cp /tmp/kustomize $(BINDIR)/kustomize
	chmod +x $(BINDIR)/kustomize
	tar zxf $(PROMTOOL_DLPATH) -C /tmp --strip-components=1 prometheus-$(PROMTOOL_VERSION).linux-amd64/promtool
	cp /tmp/promtool $(BINDIR)/promtool
	tar zxf $(TELEPORT_DLPATH) -C /tmp --strip-component=1 teleport/tsh
	cp /tmp/tsh $(BINDIR)/tsh
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

.PHONY:	validation test-alert-rules kustomize-check code-check test \
		dctest dctest-ceph dctest-reboot dctest-upgrade setup-download setup clean
