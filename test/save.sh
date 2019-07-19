#!/bin/sh

for boot in boot-0 boot-1 boot-2; do
  ./dcssh cybozu@${boot} sudo systemctl stop cke.service
done

pmctl snapshot save init

