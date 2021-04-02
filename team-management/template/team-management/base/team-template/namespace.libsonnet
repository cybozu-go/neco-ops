function(team, namespace) [
  {
    apiVersion: 'v1',
    kind: 'Namespace',
    metadata: {
      name: namespace,
      [if team == 'maneki' && namespace == 'maneki' then 'labels']: {
        'app.kubernetes.io/name': 'maneki',
      },
    },
  },
  {
    apiVersion: 'rbac.authorization.k8s.io/v1',
    kind: 'RoleBinding',
    metadata: {
      name: if team == 'maneki' && namespace == 'app-monitoring' then 'monitoring-role-binding' else team + '-role-binding',
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
