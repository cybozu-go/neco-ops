local utility = import '../utility.libsonnet';
local common_template = import 'base/common/main.libsonnet';
local kustomization_template = import 'base/kustomization.libsonnet';
local team_template = import 'base/team-template/main.libsonnet';
function(settings)
  local teams = utility.get_teams(settings);
  local all_namespaces = utility.get_all_namespaces(settings);
  local common_files = utility.prefix_file_names('base/common', common_template(teams));
  local team_files = [utility.prefix_file_names('base/' + x, team_template(x, utility.get_team_namespaces(settings, x), all_namespaces)) for x in teams];
  utility.union_map([common_files] + team_files + [{
    'base/kustomization.yaml': kustomization_template(teams),
  }])
