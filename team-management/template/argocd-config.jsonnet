local argocd_config_template = import 'argocd-config/main.libsonnet';
local settings = import 'settings.json';
local utility = import 'utility.libsonnet';
utility.prefix_file_names('argocd-config', argocd_config_template(settings))
