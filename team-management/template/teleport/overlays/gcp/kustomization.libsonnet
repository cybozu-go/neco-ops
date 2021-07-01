function(teams) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: [
    '../../base',
    'certificate.yaml',
  ],
  patchesStrategicMerge: std.set([
    'deployment.yaml',
    'statefulset.yaml',
    'apps/vmalertmanager-largeset.yaml',
    'apps/vmalertmanager-smallset.yaml',
  ] + std.map(function(x) 'nodes/' + x + '.yaml', teams + ['neco'])),
  images: [
    {
      name: 'quay.io/gravitational/teleport-ent',
      newName: 'quay.io/gravitational/teleport',
    },
  ],
}]
