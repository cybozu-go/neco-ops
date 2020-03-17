NODES=$(kubectl get nodes --no-headers | awk '{ print $1 }'); \
for n in $NODES; do ckecli ssh cybozu@$n "sudo rm -rf /var/lib/rook"; done
