#!/usr/bin/env bash

#
# Setup a private PKI a.k.a internal CA for testing-purpose
# For anyone interested in details, the following references would be helpful:
# - https://coreos.com/os/docs/latest/generate-self-signed-certificates.html#generate-ca-and-certificates
# - https://technedigitale.com/archives/639
#

GO111MODULE=off go get -v -u github.com/cloudflare/cfssl/cmd/cfssl
GO111MODULE=off go get -v -u github.com/cloudflare/cfssl/cmd/cfssljson

dir=$(cd $(dirname $0); pwd)

mkdir -p $dir/pki

cd $dir/pki

cat <<EOF > ca-config.json
{
    "signing": {
        "default": {
            "expiry": "168h"
        },
        "profiles": {
            "proxy-server": {
                "expiry": "8760h",
                "usages": [
                    "signing",
                    "key encipherment",
                    "server auth"
                ]
            }
        }
    }
}
EOF

cat <<EOF > ca-csr.json
{
    "CN": "example.com's private teleport proxy CA",
    "key": {
        "algo": "rsa",
        "size": 2048
    },
    "names": [
        {
            "C": "US",
            "ST": "CA",
            "L": "San Francisco",
            "O": "My organization",
            "OU": "My organizational unit"
        }
    ]
}
EOF

cfssl gencert -initca ca-csr.json | cfssljson -bare ca -

cat <<EOF > proxy-server-csr.json
{
    "CN": "teleport-proxy-server",
    "hosts": [
        "teleport.example.com"
    ],
    "key": {
        "algo": "rsa",
        "size": 2048
    },
    "names": [
        {
            "C": "US",
            "ST": "CA",
            "L": "San Francisco",
            "O": "My organization",
            "OU": "My organizational unit"
        }
    ]
}
EOF

cfssl gencert \
  -ca=ca.pem \
  -ca-key=ca-key.pem \
  -config=ca-config.json \
  -profile=proxy-server proxy-server-csr.json | cfssljson -bare proxy-server
