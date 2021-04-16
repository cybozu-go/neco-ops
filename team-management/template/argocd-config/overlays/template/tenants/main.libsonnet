local utility = import '../../../../utility.libsonnet';
local kustomization_template = import 'kustomization.libsonnet';
local app_template = import 'template-apps.libsonnet';
local generate_app_content = function(settings, app, overlay)
  local info = utility.get_app(settings, app);
  app_template(app, info.repo, overlay, info.destinations[overlay]);
function(settings, overlay)
  local apps = utility.get_destination_apps(settings, overlay);
  {
    'kustomization.yaml': kustomization_template(apps),
  } + utility.union_map(std.map(function(x) { [x + '.yaml']: generate_app_content(settings, x, overlay) }, apps))
