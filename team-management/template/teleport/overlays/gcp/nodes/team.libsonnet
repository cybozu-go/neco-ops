function(team) [
  (if !std.member(['maneki', 'neco'], team) then { '$patch': 'delete' } else {})
  + {
    apiVersion: 'apps/v1',
    kind: 'StatefulSet',
    metadata: {
      name: 'node-' + team,
      namespace: 'teleport',
    },
  } + (if std.member(['maneki', 'neco'], team) then
         {
           spec: {
             template: {
               spec: {
                 containers: [
                   {
                     name: 'node-' + team,
                     args: [
                       '--roles=node',
                       '--labels=team=' + team,
                       '--diag-addr=0.0.0.0:3020',
                       '--insecure',
                     ],
                   },
                 ],
               },
             },
           },
         } else {}),
]
