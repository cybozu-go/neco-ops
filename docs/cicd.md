CI/CD
=====

Overview
--------

![overview](http://www.plantuml.com/plantuml/svg/fPJVQzim4CVVzLSSUMeXCRJTZyuFeoFTkZ6wbRQmbq9Hv8kZMPQCT6u8e__xv9ljQY6mj7loMVfyxuTqfxD0qbDR6o4LEG-JStputWH0MsgBw2SWdtu6vXeVDAxxJT__26LSMy3aGjFdTZ61Nm80uguYQKk3CB6et4msJRYp1xShtIaR5tJqk3baJnrmtm7YKIIwc6693B3LE-wZVMqNw2qIXhZI1kgJgax3VKflfVB1bmxcvupAQAlY3pso1uVTIJJ6RMgq5APtTkxiKfUNifa2aifOMZ1nz2BLyOjK9xkgaGOzrTBAigy-NH2Z8YqKPeLRszdxeOGStcQzlGz__4p-P9j__FkA6-yAphmpzhvWXlUyNuPtXLvKJ2sglSCokbVGYEuA6IUa6x5R3AHjoJn5-ry9nB7X4N8DnG3K3rIddC35_AexkvynoE6OQE9qAuEvzYf-vr-OLVodz331DqQgYdT2PwN2ZxNKXhUmiuGOdeRnmiSXXXoEChYK5SBLzHIKgsjDucbxvdMvegWOmaV1SSQd0kGQhM3XfLKhY8sibt4rY94SWdKLvd0ILNuJHNs7WLh5T37b3IufJIw7LndShDmQF8RMa1XUiT5rWhwEPQ0lCKb-eDAUp-5D1ZyanPGRwKchraZV5p65fZLcJ6pRqOnTZNtwFvuILulg6OvsJc_waEHmci4dvzVI5w3jqlbQadPMfD2evCx9uLq6tnpf9TyEzzLkdBjf2-TU4sTeYxOslm40)

- Template based or plain K8s manifests is stored in the neco-apps repository.
- To verify changed manifests deploy, CI runs deployment to the `neco-ops` instance in the GCP. See details in `neco-ops` section in this file.
- [Argo CD][] watches changes of this repository, then synchronize(deploy) automatically when new commit detected.
- After the deployment process finished, [Argo CD][] sends alert to the [Alertmanager][] where is running on the same cluster. Then it notifies to Slack channel and/or Email address.

`neco-ops` instance
-------------------

`neco-ops` is a Google Compute Engine instance for GitOps testing of this repository. It is automatically created and deleted by [CircleCI scheduled workflow](https://circleci.com/docs/2.0/workflows).

The following describes an example of timeline based development flow with the Kubernetes cluster in `neco-ops` instance.

1. `neco-ops`: Launch an instance early morning.
2. `neco-ops`: Construct dctest environment.
3. `neco-ops`: Download and load virtual machine snapshots from Google Cloud Storage.
4. `neco-ops`: Ready as Kubernetes cluster.
5. `Developer`: Write some code and push commits.
6. `CircleCI`: Deploy commits to `neco-ops` cluster for testing through `Argo CD`.
7. `neco-ops`: Delete an instance at night.
8. `CircleCI`: CI does not work because of no `neco-ops` cluster in this time.
9. Go back to 1. tomorrow.

CD of each cluster
------------------

See details of the deployment step in [deploy.md](deploy.md).

- stage: watch `argocd-config/overlays/stage` in **master HEAD** branch. All changes of `master` are always deployed to staging cluster.
- bk: TBD
- prod: watch `argocd-config/overlays/prod` in **release HEAD** branch. To deploy changes for a production cluster.

[Argo CD]: https://github.com/argoproj/argo-cd
[Alertmanager]: https://prometheus.io/docs/alerting/alertmanager/
