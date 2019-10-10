#! /bin/bash -e

curl --data build_parameters[CIRCLE_JOB]=merge-stage \
"https://circleci.com/api/v1.1/project/github/cybozu-private/neco-apps-secret/tree/master?circle-token=${CIRCLE_API_TOKEN}"
