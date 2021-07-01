function(apps, teams) [
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
      roles: [
        {
          groups: (
            if app_name == 'tenant-apps' then
              ['cybozu-private:' + team for team in teams]
            else
              ['cybozu-private:' + apps[app_name].team]
          ),
          name: app_name,
          policies: [
            std.format('p, proj:tenant-app-of-apps:%(name)s, applications, get, tenant-app-of-apps/%(name)s, allow', { name: app_name }),
            std.format('p, proj:tenant-app-of-apps:%(name)s, applications, sync, tenant-app-of-apps/%(name)s, allow', { name: app_name }),
          ],
        }
        for app_name in std.objectFields(apps)
        if apps[app_name].team != '' || app_name == 'tenant-apps'
      ],
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
      clusterResourceWhitelist: [
        {
          group: 'apiextensions.k8s.io',
          kind: 'CustomResourceDefinition',
        },
        {
          group: '',
          kind: 'Namespace',
        },
        {
          group: 'rbac.authorization.k8s.io',
          kind: 'ClusterRole',
        },
        {
          group: 'rbac.authorization.k8s.io',
          kind: 'ClusterRoleBinding',
        },
      ],
      orphanedResources: {
        warn: false,
      },
      roles: [
        {
          name: 'admin',
          groups: std.set([
            'cybozu-private:csa',
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
