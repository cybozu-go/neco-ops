local neco_admission_template = import 'neco-admission/base/main.libsonnet';
local settings = import 'settings.json';
local utility = import 'utility.libsonnet';
utility.prefix_file_names('neco-admission/base', neco_admission_template(settings))
