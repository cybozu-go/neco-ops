List of Application Sync Waves
====================================

The sync order of applications can be managed with the `argocd.argoproj.io/sync-wave` annotation.

| Application          | Wave |
| -------------------- | ---- |
| namespaces           | 1    |
| argocd               | 2    |
| local-pv-provisioner | 3    |
| secrets              | 3    |
| cert-manager         | 4    |
| external-dns         | 4    |
| metallb              | 4    |
| ingress              | 5    |
| teleport             | 5    |
| topolvm              | 5    |
| elastic              | 6    |
| monitoring           | 6    |
| network-policy       | 7    |
| team-management      | 8    |
| neco-admission       | 8    |
| argocd-ingress       | 8    |
| bmc-reverse-proxy    | 8    |
| developer-apps       | 9    |
