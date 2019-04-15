#!/bin/bash

set -euf -o pipefail

# Build the controller and its end-to-end tests.
make build.local build/e2e

# Forward API server calls to the stups-test cluster.
zkubectl login stups-test
zkubectl proxy&
# Listens on 127.0.0.1:8001.

# Generate a controller ID for this run.
controllerId=ssc-e2e-$(dd if=/dev/urandom bs=8 count=1 2>/dev/null | hexdump -e '"%x"')

# We'll store the controller logs in a separate file to keep stdout clean.
controllerLog=$(mktemp /tmp/ssc-log-XXXXX.log)
echo ">>> Writing controller logs in $controllerLog"

# Find and run the controller locally.
sscPath=$(find build/ -name "stackset-controller" | head -n 1)
command $sscPath --apiserver=http://127.0.0.1:8001 --controller-id=$controllerId 2>$controllerLog&

# Create the Kubernetes namespace to be used for this test run.
zkubectl create ns $controllerId

# Run the end-to-end tests against the controller we just deployed.
# -count=1 disables go test caching.
env E2E_NAMESPACE=$controllerId \
    CLUSTER_DOMAIN=stups-test.zalan.do \
    CONTROLLER_ID=$controllerId \
    KUBECONFIG=$HOME/.kube/config \
    go test -parallel 64 -v -failfast -count=1 \
        github.com/zalando-incubator/stackset-controller/cmd/e2e \
    || true

# Delete the test namespace.
zkubectl delete ns $controllerId

# Kill all background jobs.
echo "Jobs to kill:"
jobs
pkill stackset-contro
pkill -f kubectl
