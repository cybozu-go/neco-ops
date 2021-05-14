OVERLAY ?= gcp
BOOT0 = 10.72.48.0
BOOT1 = 10.72.48.1
BOOT2 = 10.72.48.2
GINKGO = env $(GOPATH)/bin/ginkgo --failFast -v
SUITE ?= bootstrap
ifeq ($(SUITE), run)
	GINKGO += -p
endif
SSH_PRIVKEY = $(NECO_DIR)/dctest/dctest_key
CIRCLE_BUILD_NUM ?= -$(USER)
TEST_ID := test$(CIRCLE_BUILD_NUM)
BASE_BRANCH = main
COMMIT_ID = $(shell git rev-parse HEAD)
SUDO = sudo
WGET=wget --retry-connrefused --no-verbose
NUM_DASHBOARD = $(shell KUSTOMIZE_ENABLE_ALPHA_COMMANDS=true ./bin/kustomize cfg count ../monitoring/base/grafana-operator/dashboards | \
	grep GrafanaDashboard | cut -d' ' -f2)
export BOOT0 BOOT1 BOOT2 GINKGO SSH_PRIVKEY TEST_ID COMMIT_ID NUM_DASHBOARD SUDO

# Follow Argo CD installed kustomize version
# https://github.com/cybozu/neco-containers/blob/main/argocd/Dockerfile#L22
KUSTOMIZE_VERSION := 3.7.0
PROMTOOL_VERSION := 2.24.1
TELEPORT_VERSION := 5.2.1
KUBERNETES_VERSION := 1.19.7
ARGOCD_VERSION := 1.8.3

# Cache
ROOT_DIR := $(shell git rev-parse --show-toplevel)
DOWNLOAD_DIR := $(ROOT_DIR)/download
KUSTOMIZE_DLPATH := $(DOWNLOAD_DIR)/kustomize-v$(KUSTOMIZE_VERSION).tar.gz
PROMTOOL_DLPATH := $(DOWNLOAD_DIR)/prometheus-v$(PROMTOOL_VERSION).tar.gz
TELEPORT_DLPATH := $(DOWNLOAD_DIR)/teleport-v$(TELEPORT_VERSION).tar.gz

BINDIR := $(abspath $(CURDIR)/bin)
KUBECTL := $(BINDIR)/kubectl
KUSTOMIZE := $(BINDIR)/kustomize
PROMTOOL := $(BINDIR)/promtool
TSH := $(BINDIR)/tsh
ARGOCD := $(BINDIR)/argocd

install.yaml: $(shell find ../argocd/base)
	$(KUSTOMIZE) build ../argocd/base/ > install.yaml

validation:
	env SSH_PRIVKEY= go test -v -count 1 -run 'TestValidation' .

.PHONY: test-alert-rules
test-alert-rules: test-vmalert-rules

VMRULESDIR := ../monitoring/base/victoriametrics/rules
.PHONY: test-vmalert-rules
test-vmalert-rules:
	rm -rf $(VMRULESDIR)/converted
	mkdir $(VMRULESDIR)/converted
	for F in $$(cd $(VMRULESDIR); ls *rule.yaml); do \
		go run vmrule-convert/vmrule-convert.go < $(VMRULESDIR)/$$F > $(VMRULESDIR)/converted/$$F ; \
	done
	$(PROMTOOL) test rules vmalert_test/*.yaml

code-check: test-tools
	test -z "$$(gofmt -s -l . | grep -v -E '^neco-apps' | tee /dev/stderr)"
	staticcheck ./...
	nilerr ./...
	test -z "$$(custom-checker -restrictpkg.packages=html/template,log ./... 2>&1 | tee /dev/stderr)"
	go vet ./...

test: code-check install.yaml test-alert-rules validation

dctest: install.yaml
	PATH=$(BINDIR):$$PATH OVERLAY=$(OVERLAY) SUITE=$(SUITE) ./test.sh

dctest-reboot: install.yaml
	PATH=$(BINDIR):$$PATH OVERLAY=$(OVERLAY) REBOOT=1 SUITE=$(SUITE) ./test.sh

dctest-upgrade: install.yaml
	if [  "$(SUITE)" = "prepare" ]; then \
		git worktree remove /tmp/neco-apps; \
		git worktree add /tmp/neco-apps $(BASE_BRANCH) && \
		cp account.json zerossl-secret-resource.json /tmp/neco-apps/test && \
		if grep -q BOOTSTRAP /tmp/neco-apps/test/env.go; then \
			$(MAKE) -C /tmp/neco-apps/test setup dctest BOOTSTRAP=1 COMMIT_ID=$(BASE_BRANCH); \
		else \
			$(MAKE) -C /tmp/neco-apps/test setup dctest SUITE=bootstrap COMMIT_ID=$(BASE_BRANCH); \
		fi && \
		cp /tmp/neco-apps/test/argocd-password.txt ./; \
	fi
	PATH=$(BINDIR):$$PATH OVERLAY=$(OVERLAY) UPGRADE=1 SUITE=$(SUITE) ./test.sh

.PHONY: setup-download
setup-download:
	if [ -z "$$(which wget)" ]; then \
		$(SUDO) apt-get update && $(SUDO) apt-get -y install wget; \
	fi
	mkdir -p $(BINDIR)
	mkdir -p $(DOWNLOAD_DIR)

$(KUBECTL):
	$(MAKE) setup-download
	$(WGET) -O $@ https://storage.googleapis.com/kubernetes-release/release/v$(KUBERNETES_VERSION)/bin/linux/amd64/kubectl
	chmod +x $@

$(KUSTOMIZE):
	$(MAKE) setup-download
	$(WGET) -O $(KUSTOMIZE_DLPATH) https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv${KUSTOMIZE_VERSION}/kustomize_v$(KUSTOMIZE_VERSION)_linux_amd64.tar.gz
	tar zxf $(KUSTOMIZE_DLPATH) -C $(BINDIR)

$(PROMTOOL):
	$(MAKE) setup-download
	$(WGET) -O $(PROMTOOL_DLPATH) https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).linux-amd64.tar.gz
	tar zxf $(PROMTOOL_DLPATH) -C $(BINDIR) --strip-components=1 prometheus-$(PROMTOOL_VERSION).linux-amd64/promtool

$(TSH):
	$(MAKE) setup-download
	$(WGET) -O $(TELEPORT_DLPATH) https://get.gravitational.com/teleport-v$(TELEPORT_VERSION)-linux-amd64-bin.tar.gz
	tar zxf $(TELEPORT_DLPATH) -C $(BINDIR) --strip-component=1 teleport/tsh

$(ARGOCD):
	$(MAKE) setup-download
	$(WGET) -O $(BINDIR)/argocd https://github.com/argoproj/argo-cd/releases/download/v$(ARGOCD_VERSION)/argocd-linux-amd64
	chmod +x $@

.PHONY: setup
setup: $(KUBECTL) $(KUSTOMIZE) $(PROMTOOL) $(TSH) $(ARGOCD)
	go install github.com/onsi/ginkgo/ginkgo@latest

.PHONY: test-tools
test-tools: staticcheck nilerr custom-checker

.PHONY: staticcheck
staticcheck:
	if ! which staticcheck >/dev/null; then \
		env GOFLAGS= go install honnef.co/go/tools/cmd/staticcheck@latest; \
	fi

.PHONY: nilerr
nilerr:
	if ! which nilerr >/dev/null; then \
		env GOFLAGS= go install github.com/gostaticanalysis/nilerr/cmd/nilerr@latest; \
	fi

.PHONY: custom-checker
custom-checker:
	if ! which custom-checker >/dev/null; then \
		env GOFLAGS= go install github.com/cybozu/neco-containers/golang/analyzer/cmd/custom-checker@latest; \
	fi

.PHONY: clean
clean:
	rm -f install.yaml
	rm -rf $(BINDIR)
	rm -rf $(DOWNLOAD_DIR)

.PHONY:	validation test-alert-rules code-check test \
		dctest dctest-reboot dctest-upgrade
