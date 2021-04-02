#!/bin/sh
# usage: ./add_team.sh team
yq eval ". *+ {\"$1\": []}" settings.json -j | jq "with_entries(.value |= sort)" > /tmp/settings.json
mv /tmp/settings.json settings.json
