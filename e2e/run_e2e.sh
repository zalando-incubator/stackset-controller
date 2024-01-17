#!/bin/bash

set -euf -o pipefail
shopt -s nullglob

CLUSTER_DOMAIN=${CLUSTER_DOMAIN:-""}
CLUSTER_DOMAIN_INTERNAL=${CLUSTER_DOMAIN_INTERNAL:-""}
CLUSTER_NAME=${CLUSTER_NAME:-""}
# Set TEST_NAME to run a single test
# eg.: TEST_NAME=TestIngressToRouteGroupSwitch ./e2e/run_e2e.sh
TEST_NAME=${TEST_NAME:-""}
TEST_ARGS=${@:-""}
CONTROLLER_ID="ssc-e2e-$(dd if=/dev/urandom bs=8 count=1 2>/dev/null | hexdump -e '"%x"')"

if [[ -z "${CLUSTER_DOMAIN}" ]]; then
  echo "Please specify the cluster domain via CLUSTER_DOMAIN."
  exit 0
fi

if [[ -z "${CLUSTER_DOMAIN_INTERNAL}" ]]; then
  echo "Please specify the cluster domain via CLUSTER_DOMAIN_INTERNAL."
  exit 0
fi

if [[ -z "${CLUSTER_NAME}" ]]; then
  echo "Please specify the name  of the cluster via CLUSTER_NAME."
  exit 0
fi

# Build the controller and its end-to-end tests.
make build.local build/e2e

sed -ne '/stackset-controller-config/{:a' -e 'n;p;ba' -e '}' \
  e2e/apply/config.yaml > /tmp/stackset-controller-config.yaml

cleanup() {
    zkubectl delete namespace $CONTROLLER_ID --context $CLUSTER_NAME --wait=false
    kill -- -$$
}
trap cleanup EXIT

# Forward API server calls to the stups-test cluster.
zkubectl login $CLUSTER_NAME
zkubectl proxy&
# Listens on 127.0.0.1:8001.

# Generate a controller ID for this run.

# We'll store the controller logs in a separate file to keep stdout clean.
controllerLog="/tmp/ssc-log-$(date +%s).log"
echo ">>> Writing controller logs in $controllerLog"

# Find and run the controller locally.
sscPath=$(find build/ -name "stackset-controller" | head -n 1)
command $sscPath --apiserver=http://127.0.0.1:8001 \
  --ingress-source-switch-ttl="1m" \
  --enable-traffic-segments \
  --annotated-traffic-segments \
  --sync-ingress-annotation=example.org/i-haz-synchronize \
  --sync-ingress-annotation=teapot.org/the-best \
  --enable-configmap-support \
  --enable-routegroup-support \
  --cluster-domain=${CLUSTER_DOMAIN} \
  --cluster-domain=${CLUSTER_DOMAIN_INTERNAL} \
  --controller-id=$CONTROLLER_ID 2>$controllerLog&

# Create the Kubernetes namespace to be used for this test run.
zkubectl create ns $CONTROLLER_ID

test_args="-test.v"
if [[ -z "${TEST_NAME}" ]]; then
  test_args="${test_args} ${TEST_ARGS} -test.parallel 64"
else
  test_args="${test_args} ${TEST_ARGS} -test.parallel 1 -test.run=${TEST_NAME}"
fi

# Run the end-to-end tests against the controller we just deployed.
# -count=1 disables go test caching.
env E2E_NAMESPACE=$CONTROLLER_ID \
    CONTROLLER_ID=$CONTROLLER_ID \
    KUBECONFIG=$HOME/.kube/config \
    build/e2e ${test_args} || true
