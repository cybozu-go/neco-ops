#!/bin/sh
# usage: ./add_repo.sh team repo
yq eval ". * {\"repos\": {\"$2\": \"$1\"}}" settings.json -j > /tmp/settings.json
mv /tmp/settings.json settings.json
make format-settings > /dev/null
