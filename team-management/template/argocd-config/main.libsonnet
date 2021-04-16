local utility = import '../utility.libsonnet';
local overlays_template = import 'overlays/template/tenants/main.libsonnet';
function(settings)
  utility.union_map(std.map(function(x) utility.prefix_file_names('overlays/' + x + '/tenants', overlays_template(settings, x)), ['osaka0', 'stage0', 'tokyo0']))
