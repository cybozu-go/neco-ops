function(team, namespaces) [
  {
    apiVersion: 'kustomize.config.k8s.io/v1beta1',
    kind: 'Kustomization',
    resources: std.set(
      [
        'project.yaml',
        'elastic-serviceaccount.yaml',
      ] +
      std.map(function(x) x + '.yaml', namespaces)
    ),
    commonLabels: {
      team: team,
    },
  },
]
