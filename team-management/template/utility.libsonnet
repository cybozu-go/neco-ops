{
  // union_map transforms
  // [
  //   { "a": "value a" },
  //   { "b": "value b" },
  // ]
  // into
  // {
  //   "a": "value a",
  //   "b": "value b",
  // }
  union_map(arr)::
    std.foldl(function(x, y) x + y, arr, {}),

  // prefix_file_names_array transforms
  // {
  //   "path/to/file1.yaml": "file 1 content in JSON",
  //   "path/to/file2.yaml": "file 2 content in JSON"
  // }
  // into
  // [
  //   { "prefix/path/to/file1.yaml": "file 1 content in JSON" },
  //   { "prefix/path/to/file2.yaml": "file 2 content in JSON" },
  // ]
  prefix_file_names_array(prefix, files)::
    std.objectValues(std.mapWithKey(function(x, y) { [prefix + '/' + x]: y }, files)),

  // prefix_file_names transforms
  // {
  //   "path/to/file1.yaml": "file 1 content in JSON",
  //   "path/to/file2.yaml": "file 2 content in JSON"
  // }
  // into
  // {
  //   "prefix/path/to/file1.yaml": "file 1 content in JSON",
  //   "prefix/path/to/file2.yaml": "file 2 content in JSON"
  // }
  prefix_file_names(prefix, files)::
    self.union_map(self.prefix_file_names_array(prefix, files)),

  // get_teams retrieves the array of teams from settings.
  get_teams(settings)::
    std.objectFields(settings),

  // get_team_namespaces retrieves the array of namespaces associated to a team.
  get_team_namespaces(settings, team)::
    settings[team],

  // get_all_namespaces retrieves the array of all namespaces associated to the tenant teams.
  get_all_namespaces(settings)::
    std.flattenArrays(std.map(function(x) self.get_team_namespaces(settings, x), self.get_teams(settings))),
}
