//  This jsonnet file generates a dictionary consisting file path and their content.
//  {
//      "path/to/file1.yaml": "file 1 content in JSON",
//      "path/to/file2.yaml": "file 2 content in JSON"
//  }
//  'make all' builds the jsonnet output, iterate over each file, transform the file content into YAML and place it onto appropriate places.
local settings = import 'settings.json';
local team_template = import 'team-management/main.libsonnet';
local utility = import 'utility.libsonnet';
utility.prefix_file_names('team-management', team_template(settings))
