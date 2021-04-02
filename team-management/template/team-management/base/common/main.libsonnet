local project_template = import 'project.libsonnet';
function(teams) {
  'project.yaml': project_template(teams),
}
