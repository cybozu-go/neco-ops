How to write Kubernetes application manifests
=============================================

## Namespace

### Name

- Administrators should not create namespace starting with `app-` as the prefix is reserved for tenants.

### Labels

- All namespaces should have `team=xxx` labels to clarify their owner.
- To skip validation/mutation webhook, administrators can use the following special annotations
  - `mpod.kb.io/ignore: "true"`: With this label, [PodMutator](https://github.com/cybozu/neco-containers/blob/master/admission/README.md#podmutator) is ignored. This label is necessary when the pods in the namespace are required to start without neco-admission webhooks.
  - `vnetworkpolicy.kb.io/ignore: "true"`: This label enables to ignore [CalicoNetworkPolicyValidator](https://github.com/cybozu/neco-containers/blob/master/admission/README.md#caliconetworkpolicyvalidator). Using this label makes it possible for administrators to set high prioritized network policies for namespaces.
  - `topolvm.cybozu.com/webhook: ignore`: This label disables using the Topolvm webhook used for persistent volumes provided by Topolvm. Administrators should use this label for the namespaces which should be independent of Topolvm. See more details about the label [here](https://github.com/topolvm/topolvm/blob/master/deploy/README.md#protect-system-namespaces-from-topolvm-webhook).

### Annotations

- Namespaces can add the following annotations.
  - `coil.cybozu.com/pool: <address pool name>`: This annotation allows using the specified address pool. See more details [here](https://github.com/cybozu-go/coil/blob/master/docs/usage.md#using-non-default-pools).



## Secrets

- Administrators should not add sensitive secrets to this repository.
