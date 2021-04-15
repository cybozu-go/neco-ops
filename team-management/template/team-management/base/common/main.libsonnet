local utility = import '../../../utility.libsonnet';
local elastic_template = import 'elastic-rbac.libsonnet';
local project_template = import 'project.libsonnet';
function(settings)
  local teams = utility.get_teams(settings);
  {
    'elastic-rbac.yaml': elastic_template(settings),
    'project.yaml': project_template(teams),
  }
