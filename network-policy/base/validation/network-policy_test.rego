package kubernetes.test_admission

import data.kubernetes.admission

test_allow_creating_network_policy_in_system_namespaces {
	count(admission.deny) == 0 with input as {"request": {
		"kind": {
			"kind": "NetworkPolicy",
			"group": "crd.projectcalico.org",
		},
		"operation": "CREATE",
		"namespace": "argocd",
		"object": {
			"metadata": {"name": "foo"},
			"spec": {},
		},
	}}
}

test_deny_creating_high_priority_network_policy {
	count(admission.deny) > 0 with input as {"request": {
		"kind": {
			"kind": "NetworkPolicy",
			"group": "crd.projectcalico.org",
			"version": "v1",
		},
		"operation": "CREATE",
		"namespace": "foo",
		"object": {
			"metadata": {"name": "foo"},
			"spec": {"order": 100},
		},
	}}
}

test_deny_creating_high_priority_network_policy_with_old_version {
	count(admission.deny) > 0 with input as {"request": {
		"kind": {
			"kind": "NetworkPolicy",
			"group": "crd.projectcalico.org",
			"version": "v1beta1",
		},
		"operation": "CREATE",
		"namespace": "foo",
		"object": {
			"metadata": {"name": "foo"},
			"spec": {"order": 100},
		},
	}}
}

test_deny_updating_high_priority_network_policy {
	count(admission.deny) > 0 with input as {"request": {
		"kind": {
			"kind": "NetworkPolicy",
			"group": "crd.projectcalico.org",
		},
		"operation": "UPDATE",
		"namespace": "foo",
		"object": {
			"metadata": {"name": "foo"},
			"spec": {"order": 100},
		},
	}}
}

test_allow_creating_low_priority_network_policy {
	count(admission.deny) == 0 with input as {"request": {
		"kind": {
			"kind": "NetworkPolicy",
			"group": "crd.projectcalico.org",
		},
		"operation": "CREATE",
		"namespace": "foo",
		"object": {
			"metadata": {"name": "foo"},
			"spec": {"order": 2000},
		},
	}}
}
