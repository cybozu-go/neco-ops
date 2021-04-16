#!/bin/sh
# usage: ./add_app.sh app team repo
yq eval ". * {\"apps\": {\"$1\": {\"team\": \"$2\", \"repo\": \"$3\"}}}" settings.json -j > /tmp/settings.json
mv /tmp/settings.json settings.json
make format-settings > /dev/null
