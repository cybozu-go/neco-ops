function(teams) [{
  apiVersion: 'kustomize.config.k8s.io/v1beta1',
  kind: 'Kustomization',
  resources: std.set([
    './common',
    './neco',
    './neco-readonly',
    './sandbox',
  ] + std.map(function(x) './' + x, teams)),
}]
