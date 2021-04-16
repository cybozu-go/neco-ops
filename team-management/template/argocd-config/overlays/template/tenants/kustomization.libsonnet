function(apps) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: std.map(function(x) x + '.yaml', apps),
}]
