#!/bin/sh
# usage: ./add_team.sh team
yq eval ". *+ {\"namespaces\": {\"$1\": []}}" settings.json -j > /tmp/settings.json
mv /tmp/settings.json settings.json
make format-settings > /dev/null
