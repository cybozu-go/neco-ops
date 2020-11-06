#!/bin/bash -e

if [ -n "$SECRET_GITHUB_TOKEN" ]; then
    GIT_USER=cybozu-neco
    GIT_URL="https://${GIT_USER}:${SECRET_GITHUB_TOKEN}@github.com/cybozu-private/neco-apps-secret"

    if [ "${CIRCLE_BRANCH}" != "release" -a "${CIRCLE_BRANCH}" != "stage" ]; then
        BRANCH="master"
    else
        BRANCH=${CIRCLE_BRANCH}
    fi

    rm -rf ./neco-apps-secret
    git clone -b $BRANCH $GIT_URL neco-apps-secret 2> /dev/null

    ./bin/kustomize build ./neco-apps-secret/overlays/osaka0 > expected-secret-osaka0.yaml
    ./bin/kustomize build ./neco-apps-secret/overlays/stage0 > expected-secret-stage0.yaml
    ./bin/kustomize build ./neco-apps-secret/overlays/tokyo0 > expected-secret-tokyo0.yaml
    ./bin/kustomize build ../secrets/base > current-secret.yaml

elif [ -n "$SECRET_DIR" ]; then
    # By dir
    ./bin/kustomize build ${SECRET_DIR}/overlays/osaka0 > expected-secret-osaka0.yaml
    ./bin/kustomize build ${SECRET_DIR}/overlays/stage0 > expected-secret-stage0.yaml
    ./bin/kustomize build ${SECRET_DIR}/overlays/tokyo0 > expected-secret-tokyo0.yaml
    ./bin/kustomize build ../secrets/base > current-secret.yaml

else
    echo "Error: Please set env of SECRET_DIR."
    exit 2
fi
