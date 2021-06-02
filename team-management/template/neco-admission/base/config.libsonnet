local utility = import '../../utility.libsonnet';
function(settings) [{
  ArgoCDApplicationValidator: {
    rules: std.set(
      [
        {
          repository: 'https://github.com/cybozu-go/neco-apps.git',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://prometheus-community.github.io/helm-charts',
          projects: [
            'default',
          ],
        },
        {
          repository: 'https://github.com/cybozu-private/neco-apps-secret.git',
          projects: [
            'default',
          ],
        },
      ] + std.map(function(x) {
        repository: utility.get_app(settings, x).repo,
        projects: if x == 'tenant-apps' then [
          'tenant-apps',
          'tenant-app-of-apps',
        ] else std.set([utility.get_app(settings, x).team, 'maneki', 'tenant-app-of-apps']),
      }, utility.get_apps(settings)) +
      std.map(function(x) {
        repository: x,
        projects: std.set([utility.get_repo(settings, x), 'maneki', 'tenant-app-of-apps']),
      }, utility.get_repos(settings)),
      function(x) x.repository
    ),
  },
}]
