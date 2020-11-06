#!/bin/sh -ex

for dir in ${KUSTOMIZATION_DIRS}; do
    ./bin/kustomize build ${dir} >/dev/null
done
