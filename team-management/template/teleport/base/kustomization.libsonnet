function(teams) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: [
    'deployment.yaml',
    'rbac.yaml',
    'service.yaml',
    'serviceaccount.yaml',
    'statefulset.yaml',
    'apps',
    'nodes',
  ],
  configMapGenerator: [
    {
      name: 'teleport-role',
      namespace: 'teleport',
      files: std.set([
        'conf/admin-role.yaml',
        'conf/neco-readonly-role.yaml',
      ] + std.map(function(x) 'conf/' + x + '-role.yaml', teams)),
    },
  ],
  images: [
    {
      name: 'quay.io/gravitational/teleport-ent',
      newTag: '6.2.0',
    },
  ],
}]
