#! /bin/sh -ex
PROJECT=neco-test
ZONE=gcp0
GCLOUD_DNS="gcloud dns --project=$PROJECT record-sets"

${GCLOUD_DNS} list --zone=$ZONE --name="teleport.gcp0.dev-ne.co." --type="CNAME" | tail -n +2 > zone.txt
lines=$(cat zone.txt | wc -l)
if [ $lines -eq 0 ]; then
   gcloud dns --project=$PROJECT record-sets transaction start --zone=$ZONE
   gcloud dns --project=$PROJECT record-sets transaction add teleport-proxy.teleport.svc.cluster.local. --name=teleport.gcp0.dev-ne.co. --ttl=300 --type=CNAME --zone=$ZONE
   gcloud dns --project=$PROJECT record-sets transaction execute --zone=$ZONE
fi
