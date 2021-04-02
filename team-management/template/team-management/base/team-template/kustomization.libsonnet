function(team, namespaces) [
  {
    apiVersion: 'kustomize.config.k8s.io/v1beta1',
    kind: 'Kustomization',
    resources: std.set(
      ['project.yaml'] +
      std.map(function(x) x + '.yaml', namespaces) +
      if team == 'ept' || team == 'maneki' then ['elastic-serviceaccount.yaml'] else []
    ),
    commonLabels: {
      team: team,
    },
  },
]
