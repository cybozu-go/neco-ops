#! /bin/sh -ex
PROJECT=neco-test
ZONE=gcp0
GCLOUD_DNS="gcloud dns --project=$PROJECT record-sets"
ECHO_TEST=""
MAX_LINES=500

# this script cannot handle mutiple value RRset (e.g. multiple address to same host name)

${GCLOUD_DNS} list --zone=$ZONE --filter="NOT TYPE=(SOA NS)" | tail -n +2 > zone.txt
lines=$(cat zone.txt | wc -l)
if [ $lines -eq 0 ]; then
  exit
fi
loops=$(((lines - 1) / MAX_LINES + 1))
for i in $(seq $loops); do
  ${ECHO_TEST} ${GCLOUD_DNS} transaction start --zone=$ZONE
  tail -n +$(((i-1)*MAX_LINES+1)) zone.txt | head -n ${MAX_LINES} | while read name type ttl data; do
      ${ECHO_TEST} ${GCLOUD_DNS} transaction remove "$data" --name=$name --type=$type --ttl=$ttl --zone=$ZONE
  done
  ${ECHO_TEST} ${GCLOUD_DNS} transaction execute --zone=$ZONE
done
