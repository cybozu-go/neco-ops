#!/bin/sh
# usage: ./add_namespace.sh team namespace
yq eval ". *+ {\"$1\": [\"$2\"]}" settings.json -j | jq "with_entries(.value |= sort)" > /tmp/settings.json
mv /tmp/settings.json settings.json
