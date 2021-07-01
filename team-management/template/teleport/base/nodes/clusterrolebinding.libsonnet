function() [{
  apiVersion: 'rbac.authorization.k8s.io/v1',
  kind: 'ClusterRoleBinding',
  metadata: {
    name: 'node-neco',
  },
  roleRef: {
    apiGroup: 'rbac.authorization.k8s.io',
    kind: 'ClusterRole',
    name: 'cluster-admin',
  },
  subjects: [
    {
      kind: 'ServiceAccount',
      name: 'node-neco',
      namespace: 'teleport',
    },
  ],
}]
