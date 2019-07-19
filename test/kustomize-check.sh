#!/bin/sh -ex

for dir in ${KUSTOMIZATION_DIRS}; do
    kustomize build ${dir} >/dev/null
done
