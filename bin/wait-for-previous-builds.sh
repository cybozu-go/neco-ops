#!/bin/sh -e
# This uses the same logic as https://github.com/bellkev/circle-lock-test .

parse_args () {
    while [ $# -gt 0 ]; do
        case $1 in
            -j|--job)
                shift
                job=$1
                ;;
            *)
                exit 2
                ;;
        esac
        shift  # this fails in the case of lack of args
    done
}

check_skip () {
    if [ -n "${job}" -a "${CIRCLE_JOB}" != "${job}" ]; then
        echo "Not in job ${job}.  Skipping..."
        exit 0
    fi
}

make_jq_prog () {
    local jq_filters=""

    if [ -n "${job}" ]; then
        jq_filters="${jq_filters} and .workflows.job_name == \"${job}\""
    fi

    jq_prog=".[] | select(.build_num < ${CIRCLE_BUILD_NUM} and (.status | test(\"running|pending|queued\")) ${jq_filters}) | .build_num"
}

get_builds () {
    curl -s -H "Accept: application/json" "https://circleci.com/api/v1/project/${CIRCLE_PROJECT_USERNAME}/${CIRCLE_PROJECT_REPONAME}?circle-token=${CIRCLE_TOKEN}&limit=100"
}

job=""

parse_args "$@"
check_skip
make_jq_prog

while true; do
    builds=$(get_builds | jq "${jq_prog}")
    if [ -z "${builds}" ]; then
        break
    fi
    echo "Waiting on builds:"
    echo "${builds}"
    sleep 10
done
