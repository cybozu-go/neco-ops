BOOT0 = 10.72.48.0
GINKGO = env GO111MODULE=on GOFLAGS=-mod=vendor $(GOPATH)/bin/ginkgo --failFast -v
SSH_PRIVKEY = $(realpath ./dctest_key)
TEST_ID := $(shell basename $(dir $(GOPATH)))
COMMIT_ID := $(shell git rev-parse --abbrev-ref HEAD)
KUSTOMIZATION_DIRS := $(shell find ../ -name "kustomization.yaml" -exec dirname {} \;)
PROMTOOL_VERSION := 2.9.2
export BOOT0 GINKGO SSH_PRIVKEY TEST_ID COMMIT_ID KUSTOMIZATION_DIRS

### for Go
GOFLAGS = -mod=vendor
export GOFLAGS

# Follow Argo CD installed kustomize version
# https://github.com/cybozu/neco-containers/blob/master/argocd/Dockerfile#L32
KUSTOMIZE_VERSION = 2.0.3
OPA_VERSION = 0.12.0
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

test: install.yaml validation test-alert-rules opa-test
	./test.sh

kustomize-check:
	./kustomize-check.sh

setup:
	curl -sSLf -O https://github.com/kubernetes-sigs/kustomize/releases/download/v$(KUSTOMIZE_VERSION)/kustomize_$(KUSTOMIZE_VERSION)_linux_amd64
	sudo mv kustomize_$(KUSTOMIZE_VERSION)_linux_amd64 /usr/local/bin/kustomize
	chmod +x /usr/local/bin/kustomize
	curl -sSLf -O https://github.com/open-policy-agent/opa/releases/download/v$(OPA_VERSION)/opa_linux_amd64
	sudo mv opa_linux_amd64 /usr/local/bin/opa
	chmod +x /usr/local/bin/opa
	go install github.com/onsi/ginkgo/ginkgo
	curl -sSLf https://github.com/prometheus/prometheus/releases/download/v$(PROMTOOL_VERSION)/prometheus-$(PROMTOOL_VERSION).linux-amd64.tar.gz | tar -xzvf - --strip-components=1 prometheus-$(PROMTOOL_VERSION).linux-amd64/promtool
	sudo mv promtool /usr/local/bin/promtool

save:
	./save.sh

clean:
	rm -f install.yaml

.PHONY:	validation opa-test test test-alert-rules kustomize-check setup save clean
