local utility = import '../../utility.libsonnet';
local conf_template = import 'conf/main.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local nodes_template = import 'nodes/main.libsonnet';
function(teams)
  utility.prefix_file_names('conf', conf_template(teams)) +
  utility.prefix_file_names('nodes', nodes_template(teams + ['neco'])) + {
    'kustomization.yaml': kustomization_template(teams),
  }
