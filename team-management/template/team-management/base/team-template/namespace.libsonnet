function(team, namespace) [
  {
    apiVersion: 'v1',
    kind: 'Namespace',
    metadata: {
      name: namespace,
      // This label may be unused.
      // It is written here for backward compatibility.
      labels: {
        [if team == 'maneki' && namespace == 'maneki' then 'app.kubernetes.io/name']: 'maneki',
        'hnc.x-k8s.io/excluded-namespace': 'true',
      },
    },
  },
  {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'RoleBinding',
    metadata: {
      name: team + '-role-binding',
      namespace: namespace,
    },
    roleRef: {
      apiGroup: 'rbac.authorization.k8s.io',
      kind: 'ClusterRole',
      name: 'admin',
    },
    subjects: std.set([
      {
        kind: 'Group',
        name: team,
        apiGroup: 'rbac.authorization.k8s.io',
      },
      {
        kind: 'Group',
        name: 'maneki',
        apiGroup: 'rbac.authorization.k8s.io',
      },
      {
        kind: 'ServiceAccount',
        name: 'node-maneki',
        namespace: 'teleport',
      },
      {
        kind: 'ServiceAccount',
        name: 'node-' + team,
        namespace: 'teleport',
      },
    ], function(x) x.kind + x.name),
  },
]
