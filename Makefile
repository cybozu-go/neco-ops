# Makefile to update manifests

HELM_VERSION = 3.6.0
TANKA_VERSION = 0.15.1
JSONNET_LIBS_K8S_ALPHA_VERSION = 1.20

.PHONY: all
all:
	@echo Read docs/maintenance.md for the usage

.PHONY: update-argocd
update-argocd:
	$(call get-latest-tag,argocd)
	curl -sLf -o argocd/base/upstream/install.yaml \
		https://raw.githubusercontent.com/argoproj/argo-cd/$(call upstream-tag,$(latest_tag))/manifests/install.yaml
	sed -i -E '/name:.*argocd$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' argocd/base/kustomization.yaml
	sed -i -e 's/ARGOCD_VERSION *:=.*/ARGOCD_VERSION := $(subst v,,$(call upstream-tag,$(latest_tag)))/' test/Makefile
	$(call get-latest-tag,dex)
	sed -i -E '/name:.*dex$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' argocd/base/kustomization.yaml
	$(call get-latest-tag,redis)
	sed -i -E '/name:.*redis$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' argocd/base/kustomization.yaml

.PHONY: update-bmc-reverse-proxy
update-bmc-reverse-proxy:
	$(call get-latest-tag,bmc-reverse-proxy)
	sed -i -E 's,image: quay.io/cybozu/bmc-reverse-proxy:.*$$,image: quay.io/cybozu/bmc-reverse-proxy:$(latest_tag),' bmc-reverse-proxy/base/bmc-reverse-proxy/deployment.yaml

.PHONY: update-calico
update-calico:
	$(call get-latest-tag,calico)
	curl -sfL -o network-policy/base/calico/upstream/calico-policy-only.yaml \
		https://docs.projectcalico.org/v$(shell echo $(latest_tag) | sed -E 's/^(.*)\.[[:digit:]]+\.[[:digit:]]+$$/\1/')/manifests/calico-policy-only.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' network-policy/base/kustomization.yaml

.PHONY: update-cert-manager
update-cert-manager:
	$(call get-latest-tag,cert-manager)
	curl -sLf -o cert-manager/base/upstream/cert-manager.yaml \
		https://github.com/jetstack/cert-manager/releases/download/$(call upstream-tag,$(latest_tag))/cert-manager.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' cert-manager/base/kustomization.yaml

.PHONY: update-customer-egress
update-customer-egress:
	curl -sLf -o customer-egress/base/neco/squid.yaml \
		https://raw.githubusercontent.com/cybozu-go/neco/release/etc/squid.yml
	sed -e 's/internet-egress/customer-egress/g' \
		-e 's,{{ .squid }},quay.io/cybozu/squid,g' \
		-e 's,{{ index . "cke-unbound" }},quay.io/cybozu/unbound,g' \
		-e '/nodePort: 30128/d' customer-egress/base/neco/squid.yaml > customer-egress/base/squid.yaml
	$(call get-latest-tag,squid)
	sed -i -E '/name:.*squid$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' customer-egress/base/kustomization.yaml
	$(call get-latest-tag,unbound)
	sed -i -E '/name:.*unbound$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' customer-egress/base/kustomization.yaml

.PHONY: update-eck
update-eck:
	$(call get-latest-gh,elastic/cloud-on-k8s)
	curl -sLf -o elastic/base/upstream/all-in-one.yaml https://download.elastic.co/downloads/eck/$(latest_gh)/all-in-one.yaml

.PHONY: update-external-dns
update-external-dns:
	$(call get-latest-tag,external-dns)
	curl -sLf -o external-dns/base/upstream/crd.yaml \
		https://raw.githubusercontent.com/kubernetes-sigs/external-dns/$(call upstream-tag,$(latest_tag))/docs/contributing/crd-source/crd-manifest.yaml
	sed -i -E 's,quay.io/cybozu/external-dns:.*$$,quay.io/cybozu/external-dns:$(latest_tag),' external-dns/base/deployment.yaml

.PHONY: update-grafana-operator
update-grafana-operator:
	$(call get-latest-tag,grafana-operator)
	rm -rf /tmp/grafana-operator
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/integr8ly/grafana-operator
	rm -rf monitoring/base/grafana-operator/upstream/*
	cp -r /tmp/grafana-operator/deploy/crds monitoring/base/grafana-operator/upstream
	cp -r /tmp/grafana-operator/deploy/cluster_roles monitoring/base/grafana-operator/upstream
	cp -r /tmp/grafana-operator/deploy/roles monitoring/base/grafana-operator/upstream
	cp /tmp/grafana-operator/deploy/operator.yaml monitoring/base/grafana-operator/upstream
	rm -rf /tmp/grafana-operator
	sed -i -E '/newName:.*grafana-operator$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml
	$(call get-latest-tag,grafana_plugins_init)
	sed -i -E 's/grafana-plugins-init-container-tag=.*$$/grafana-plugins-init-container-tag=$(latest_tag)/' monitoring/base/grafana-operator/operator.yaml

.PHONY: update-grafana
update-grafana:
	$(call get-latest-tag,grafana)
	sed -i -E 's/grafana-image-tag=.*$$/grafana-image-tag=$(latest_tag)/' monitoring/base/grafana-operator/operator.yaml
	sed -i -E 's,quay.io/cybozu/grafana:.*$$,quay.io/cybozu/grafana:$(latest_tag),' sandbox/overlays/gcp/grafana/statefulset.yaml

.PHONY: update-heartbeat
update-heartbeat:
	$(call get-latest-tag,heartbeat)
	sed -i -E '/name:.*heartbeat$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-kube-metrics-adapter
update-kube-metrics-adapter:
	$(call get-latest-tag,kube-metrics-adapter)
	rm -rf /tmp/kube-metrics-adapter
	cd /tmp; git clone -b $(call upstream-tag,$(latest_tag)) --depth 1 https://github.com/zalando-incubator/kube-metrics-adapter
	helm template \
		--set namespace=kube-metrics-adapter \
		--set enableExternalMetricsApi=true \
		--set service.internalPort=6443 \
		--set replicas=2 \
		/tmp/kube-metrics-adapter/docs/helm > kube-metrics-adapter/base/upstream/manifest.yaml
	rm -rf /tmp/kube-metrics-adapter
	sed -i 's/newTag: .*/newTag: $(latest_tag)/' kube-metrics-adapter/base/kustomization.yaml

.PHONY: update-kube-state-metrics
update-kube-state-metrics:
	$(call get-latest-tag,kube-state-metrics)
	rm -rf /tmp/kube-state-metrics
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/kubernetes/kube-state-metrics
	rm -f monitoring/base/kube-state-metrics/*
	cp /tmp/kube-state-metrics/examples/standard/* monitoring/base/kube-state-metrics
	rm -rf /tmp/kube-state-metrics
	sed -i -E '/newName:.*kube-state-metrics$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-local-pv-provisioner
update-local-pv-provisioner:
	$(call get-latest-tag,local-pv-provisioner)
	sed -i -E 's,image: quay.io/cybozu/local-pv-provisioner:.*$$,image: quay.io/cybozu/local-pv-provisioner:$(latest_tag),' local-pv-provisioner/base/daemonset.yaml

.PHONY: update-logging-loki
update-logging-loki:
	$(call get-latest-tag,loki)
	rm -rf /tmp/loki
	mkdir /tmp/loki
	cd /tmp/loki; \
	tk init && \
	tk env add environments/loki --namespace=logging && \
	tk env add environments/loki-canary --namespace=logging && \
	jb install github.com/grafana/loki/production/ksonnet/loki@$(call upstream-tag,$(latest_tag)) && \
	jb install github.com/grafana/loki/production/ksonnet/loki-canary@$(call upstream-tag,$(latest_tag)) && \
	jb install github.com/jsonnet-libs/k8s-alpha/$(JSONNET_LIBS_K8S_ALPHA_VERSION) && \
	echo "import 'github.com/jsonnet-libs/k8s-alpha/$(JSONNET_LIBS_K8S_ALPHA_VERSION)/main.libsonnet'" > lib/k.libsonnet

	cp logging/base/loki/main.jsonnet /tmp/loki/environments/loki/main.jsonnet
	cp logging/base/loki-canary/main.jsonnet /tmp/loki/environments/loki-canary/main.jsonnet
	rm -rf logging/base/loki/upstream/* logging/base/loki-canary/upstream/*
	cd /tmp/loki && \
	tk export $(shell pwd)/logging/base/loki/upstream/ environments/loki/ -t '!.*/consul(-sidekick)?' && \
	tk export $(shell pwd)/logging/base/loki-canary/upstream/ environments/loki-canary/

	sed -i -E '/name:.*loki$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/loki*/kustomization.yaml

	$(call get-latest-tag,memcached)
	sed -i -E '/name:.*memcached$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/loki/kustomization.yaml
	$(call get-latest-tag,memcached-exporter)
	sed -i -E '/name:.*memcached-exporter$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/loki/kustomization.yaml

.PHONY: update-logging-promtail
update-logging-promtail:
	$(call get-latest-tag,promtail)
	sed -i -E '/name:.*promtail$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/promtail/kustomization.yaml

.PHONY: update-logging-consul
update-logging-consul:
	$(call get-latest-tag,consul)
	sed -i -E '/name:.*consul$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' logging/base/consul/kustomization.yaml

.PHONY: update-machines-endpoints
update-machines-endpoints:
	$(call get-latest-tag,machines-endpoints)
	sed -i -E 's,image: quay.io/cybozu/machines-endpoints:.*$$,image: quay.io/cybozu/machines-endpoints:$(latest_tag),' bmc-reverse-proxy/base/machines-endpoints/cronjob.yaml
	sed -i -E 's,image: quay.io/cybozu/machines-endpoints:.*$$,image: quay.io/cybozu/machines-endpoints:$(latest_tag),' monitoring/base/machines-endpoints/cronjob.yaml

.PHONY: update-metallb
update-metallb:
	$(call get-latest-tag,metallb)
	rm -rf /tmp/metallb
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/metallb/metallb
	rm -f metallb/base/upstream/*
	cp /tmp/metallb/manifests/*.yaml metallb/base/upstream
	rm -rf /tmp/metallb
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' metallb/base/kustomization.yaml

.PHONY: update-moco
update-moco:
	$(call get-latest-gh,cybozu-go/moco)
	curl -sfL -o moco/base/upstream/moco.yaml \
		https://github.com/cybozu-go/moco/releases/download/$(latest_gh)/moco.yaml

.PHONY: update-neco-admission
update-neco-admission:
	$(call get-latest-tag,neco-admission)
	curl -sfL -o neco-admission/base/upstream/manifests.yaml \
		https://raw.githubusercontent.com/cybozu/neco-containers/main/admission/config/webhook/manifests.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' neco-admission/base/kustomization.yaml

.PHONY: update-prometheus-adapter
update-prometheus-adapter:
	$(call get-latest-tag,prometheus-adapter)
	sed -i -E \
		-e 's/^(          tag:).*$$/\1 $(latest_tag)/' \
		-e 's/^(    targetRevision:).*$$/\1 $(CHART_VERSION)/' \
		argocd-config/base/prometheus-adapter.yaml
	rm -rf /tmp/prometheus-adapter

.PHONY: update-pod-security-admission
update-pod-security-admission:
	$(call get-latest-gh,cybozu-go/pod-security-admission)
	curl -sfL -o pod-security-admission/base/upstream/install.yaml \
		https://github.com/cybozu-go/pod-security-admission/releases/download/$(latest_gh)/install.yaml

.PHONY: update-pushgateway
update-pushgateway:
	$(call get-latest-tag,pushgateway)
	sed -i -E '/name:.*pushgateway$$/!b;n;s/newTag:.*$$/newTag: $(latest_tag)/' monitoring/base/kustomization.yaml

.PHONY: update-pvc-autoresizer
update-pvc-autoresizer:
	$(call get-latest-gh,topolvm/pvc-autoresizer)
	rm -rf /tmp/pvc-autoresizer
	cd /tmp; git clone --depth 1 -b $(latest_gh) https://github.com/topolvm/pvc-autoresizer
	rm -rf pvc-autoresizer/base/upstream/*
	cp -r /tmp/pvc-autoresizer/config/* pvc-autoresizer/base/upstream
	rm -rf /tmp/pvc-autoresizer

.PHONY: update-s3gw
update-s3gw:
	$(call get-latest-tag,s3gw)
	sed -i -E 's,quay.io/cybozu/s3gw:.*$$,quay.io/cybozu/s3gw:$(latest_tag),' session-log/base/s3gw.yaml

.PHONY: update-sealed-secrets
update-sealed-secrets:
	$(call get-latest-tag,sealed-secrets)
	curl -sfL -o sealed-secrets/base/upstream/controller.yaml \
		https://github.com/bitnami-labs/sealed-secrets/releases/download/$(call upstream-tag,$(latest_tag))/controller.yaml
	sed -i -E 's/newTag:.*$$/newTag: $(latest_tag)/' sealed-secrets/base/kustomization.yaml

.PHONY: update-topolvm
update-topolvm:
	$(call get-latest-gh,topolvm/topolvm)
	rm -rf /tmp/topolvm
	cd /tmp; git clone --depth 1 -b $(latest_gh) https://github.com/topolvm/topolvm
	rm -rf topolvm/base/upstream/*
	cp -r /tmp/topolvm/deploy/manifests/* topolvm/base/upstream/
	rm -rf /tmp/topolvm
	sed -i -E 's/newTag:.*$$/newTag: $(patsubst v%,%,$(latest_gh))/' topolvm/base/kustomization.yaml

.PHONY: update-victoriametrics-operator
update-victoriametrics-operator:
	$(call get-latest-tag,victoriametrics-operator)
	rm -rf /tmp/operator
	cd /tmp; git clone --depth 1 -b $(call upstream-tag,$(latest_tag)) https://github.com/VictoriaMetrics/operator
	rm -rf monitoring/base/victoriametrics/upstream/*
	cp -r /tmp/operator/config/crd /tmp/operator/config/rbac monitoring/base/victoriametrics/upstream/
	rm -rf /tmp/operator
	sed -i -E 's,quay.io/cybozu/victoriametrics-operator:.*$$,quay.io/cybozu/victoriametrics-operator:$(latest_tag),' monitoring/base/victoriametrics/operator.yaml

.PHONY: update-victoriametrics
update-victoriametrics:
	$(call get-latest-tag,victoriametrics-vmalert)
	sed -i -E '/name: VM_VMALERTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmagent)
	sed -i -E '/name: VM_VMAGENTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmsingle)
	sed -i -E '/name: VM_VMSINGLEDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmselect)
	sed -i -E '/name: VM_VMCLUSTERDEFAULT_VMSELECTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vmstorage)
	sed -i -E '/name: VM_VMCLUSTERDEFAULT_VMSTORAGEDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,victoriametrics-vminsert)
	sed -i -E '/name: VM_VMCLUSTERDEFAULT_VMINSERTDEFAULT_VERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,alertmanager)
	sed -i -E '/name: VM_VMALERTMANAGER_ALERTMANAGERVERSION$$/!b;n;s/value:.*$$/value: "$(latest_tag)"/' monitoring/base/victoriametrics/operator.yaml
	$(call get-latest-tag,configmap-reload)
	sed -i -E 's,quay.io/cybozu/configmap-reload:.*$$,quay.io/cybozu/configmap-reload:$(latest_tag),' monitoring/base/victoriametrics/operator.yaml monitoring/base/victoriametrics/vmalertmanager-largeset.yaml monitoring/base/victoriametrics/vmalertmanager-smallset.yaml
	$(call get-latest-tag,prometheus-config-reloader)
	sed -i -E 's,quay.io/cybozu/prometheus-config-reloader:.*$$,quay.io/cybozu/prometheus-config-reloader:$(latest_tag),' monitoring/base/victoriametrics/operator.yaml

# usage: get-latest-tag NAME
define get-latest-tag
$(eval latest_tag := $(shell curl -sf https://quay.io/api/v1/repository/cybozu/$1/tag/ | jq -r '.tags[] | .name' | awk '/.*\..*\./ {print $$1; exit}'))
endef

# usage: upstream-tag 1.2.3.4
define upstream-tag
$(shell echo $1 | sed -E 's/^(.*)\.[[:digit:]]+$$/v\1/')
endef

# usage get-latest-gh OWNER/REPO
define get-latest-gh
$(eval latest_gh := $(shell curl -sf https://api.github.com/repos/$1/releases/latest | jq -r '.tag_name'))
endef

.PHONY: setup
setup:
	# helm
	curl -o /tmp/helm.tgz -fsL https://get.helm.sh/helm-v$(HELM_VERSION)-linux-amd64.tar.gz
	mkdir -p $$(go env GOPATH)/bin
	tar --strip-components=1 -C $$(go env GOPATH)/bin -xzf /tmp/helm.tgz linux-amd64/helm
	rm -f /tmp/helm.tgz

	# tanka
	curl -o $$(go env GOPATH)/bin/tk -fsL https://github.com/grafana/tanka/releases/download/v$(TANKA_VERSION)/tk-linux-amd64
	chmod +x $$(go env GOPATH)/bin/tk

	# jb
	go install github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb@latest
