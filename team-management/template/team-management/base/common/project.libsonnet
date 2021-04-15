function(teams) [
  {
    apiVersion: 'argoproj.io/v1alpha1',
    kind: 'AppProject',
    metadata: {
      name: 'tenant-app-of-apps',
      namespace: 'argocd',
    },
    spec: {
      sourceRepos: [
        '*',
      ],
      destinations: [
        {
          namespace: 'argocd',
          server: '*',
        },
      ],
      namespaceResourceWhitelist: [
        {
          group: 'argoproj.io',
          kind: 'Application',
        },
      ],
      orphanedResources: {
        warn: false,
      },
    },
  },
  {
    apiVersion: 'argoproj.io/v1alpha1',
    kind: 'AppProject',
    metadata: {
      name: 'tenant-apps',
      namespace: 'argocd',
    },
    spec: {
      sourceRepos: [
        '*',
      ],
      destinations: [
        {
          namespace: '*',
          server: '*',
        },
      ],
      namespaceResourceBlacklist: [
        {
          group: '',
          kind: 'ResourceQuota',
        },
        {
          group: '',
          kind: 'LimitRange',
        },
        {
          group: 'networking.k8s.io',
          kind: 'NetworkPolicy',
        },
      ],
      orphanedResources: {
        warn: false,
      },
      roles: [
        {
          name: 'admin',
          groups: std.set([
            'cybozu-private:neco',
          ] + std.map(function(x) 'cybozu-private:' + x, teams)),
          policies: [
            'p, proj:tenant-apps:admin, applications, *, tenant-apps/*, allow',
          ],
        },
      ],
    },
  },
]
