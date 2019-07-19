package kubernetes.admission

operations = {"CREATE", "UPDATE"}

system_namespaces = {"kube-system", "argocd", "external-dns", "ingress", "internet-egress", "metallb-system", "monitoring", "opa"}

deny[msg] {
	input.request.kind.kind == "NetworkPolicy"
	input.request.kind.group == "crd.projectcalico.org"
	operations[input.request.operation]
	not system_namespaces[input.request.namespace]
	input.request.object.spec.order <= 1000
	msg := "cannot create/update non-system NetworkPolicy with order <= 1000"
}
