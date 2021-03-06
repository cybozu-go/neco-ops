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
          repositoryPrefix: 'https://github.com/cybozu-private',
          projects: std.setDiff(std.set(utility.get_teams(settings) + [
            'default',
            'tenant-apps',
            'tenant-app-of-apps',
          ]), ['neco-devusers']),
        },
        {
          repositoryPrefix: 'https://github.com/garoon-private',
          projects: [
            'garoon',
            'maneki',
            'tenant-app-of-apps',
          ],
        },
      ],
      function(x) if std.objectHas(x, 'repositoryPrefix') then 'A' + x.repositoryPrefix else 'B' + x.repository
    ),
  },
}]
