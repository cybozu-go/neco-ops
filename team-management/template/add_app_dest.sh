#!/bin/sh
# usage: ./add_app_dest.sh app destination branch
yq eval ". * {\"apps\": {\"$1\": {\"destinations\": {\"$2\": \"$3\"}}}}" settings.json -j > /tmp/settings.json
mv /tmp/settings.json settings.json
make format-settings > /dev/null
