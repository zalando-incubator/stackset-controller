#!/bin/bash

set -euf -o pipefail
shopt -s nullglob

CLUSTER_DOMAIN=${CLUSTER_DOMAIN:-""}
CLUSTER_NAME=${CLUSTER_NAME:-""}

if [[ -z "${CLUSTER_DOMAIN}" ]]; then
  echo "Please specify the cluster domain via CLUSTER_DOMAIN."
  exit 0
fi

if [[ -z "${CLUSTER_NAME}" ]]; then
  echo "Please specify the name  of the cluster via CLUSTER_NAME."
  exit 0
fi

# Build the controller and its end-to-end tests.
make build.local build/e2e

# Forward API server calls to the stups-test cluster.
zkubectl login $CLUSTER_NAME
zkubectl proxy&
# Listens on 127.0.0.1:8001.

# Generate a controller ID for this run.
controllerId=ssc-e2e-$(dd if=/dev/urandom bs=8 count=1 2>/dev/null | hexdump -e '"%x"')

# We'll store the controller logs in a separate file to keep stdout clean.
controllerLog="/tmp/ssc-log-$(date +%s).log"
echo ">>> Writing controller logs in $controllerLog"

# Find and run the controller locally.
sscPath=$(find build/ -name "stackset-controller" | head -n 1)
command $sscPath --apiserver=http://127.0.0.1:8001 --controller-id=$controllerId 2>$controllerLog&

# Create the Kubernetes namespace to be used for this test run.
zkubectl create ns $controllerId

# Run the end-to-end tests against the controller we just deployed.
# -count=1 disables go test caching.
env E2E_NAMESPACE=$controllerId \
    CONTROLLER_ID=$controllerId \
    KUBECONFIG=$HOME/.kube/config \
    build/e2e -test.v -test.parallel 64 || true

# Delete the test namespace.
zkubectl delete ns $controllerId

# Kill all background jobs.
echo "Jobs to kill:"
jobs
pkill stackset-controller
pkill -f kubectl
