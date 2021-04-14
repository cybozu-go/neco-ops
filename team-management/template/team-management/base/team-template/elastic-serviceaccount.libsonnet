function(namespaces)
  std.map(function(x) {
    apiVersion: 'v1',
    kind: 'ServiceAccount',
    metadata: {
      name: 'elastic',
      namespace: x,
    },
  }, namespaces)
