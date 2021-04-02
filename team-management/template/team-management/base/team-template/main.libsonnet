local kustomization_template = import 'kustomization.libsonnet';
local namespace_template = import 'namespace.libsonnet';
local project_template = import 'project.libsonnet';
function(team, namespaces, all_teams_namespaces) {
  'kustomization.yaml': kustomization_template(team, namespaces),
  'project.yaml': if team == 'maneki' then project_template(team, std.uniq(namespaces + all_teams_namespaces)) else project_template(team, namespaces),
} + std.foldl(function(x, y) x { [y + '.yaml']: namespace_template(team, y) }, namespaces, {})
