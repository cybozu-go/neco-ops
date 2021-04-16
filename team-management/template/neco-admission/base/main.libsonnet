local config_template = import 'config.libsonnet';
function(settings)
  { 'config.yaml': config_template(settings) }
