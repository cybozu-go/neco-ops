local utility = import '../../../utility.libsonnet';
local project_template = import 'project.libsonnet';
function(settings)
  local teams = utility.get_teams(settings);
  {
    'project.yaml': project_template(settings.apps, teams),
  }
