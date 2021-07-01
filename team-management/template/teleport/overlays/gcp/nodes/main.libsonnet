local utility = import '../../../../utility.libsonnet';
local team_template = import 'team.libsonnet';
function(teams)
  utility.union_map(std.map(function(x) { [x + '.yaml']: team_template(x) }, teams + ['neco']))
