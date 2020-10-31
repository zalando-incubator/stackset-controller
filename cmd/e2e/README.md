### Running the End-To-End Tests

The following environment variables should be set:

1. `E2E_NAMESPACE` is the namespace where the tests should be run.
2. `CLUSTER_DOMAIN` is the DNS domain managed in which ingresses should be created
3. `CLUSTER_DOMAIN_INTERNAL` is the internal DNS domain managed in which ingresses should be created
4. `CONTROLLER_ID` is set so that all stacks are only managed by the controller being currently tested.
5. `KUBECONFIG` with the path to the kubeconfig file

To run the tests run the command:

```
go test -parallel $NUM_PARALLEL github.com/zalando-incubator/stackset-controller/cmd/e2e
```

Over here `$NUM_PARALLEL` can be set to a sufficiently high value which indicates how many
of the parallel type tests can be run concurrently.

#### Example - run E2E test

1. Start apiserver proxy
```
kubectl proxy
```
2. use kubectl watch to show what happens
```
watch -n 10 "kubectl get -n foo stackset,stack,ing,ep,deployment"
```
3. recreate namespace `foo` and run local build stackset-controller
```
kubectl delete namespace foo; kubectl create namespace foo
make
./build/stackset-controller --apiserver=http://127.0.0.1:8001 --controller-id=foo
```
4. rebuild e2e test and run e2e tests in `foo` namespace
```
rm -f build/e2e; make build/e2e
CLUSTER_DOMAIN=example.org CLUSTER_DOMAIN_INTERNAL=ingress.cluster.local CLUSTER_NAME=example E2E_NAMESPACE=foo CONTROLLER_ID=foo KUBECONFIG=$HOME/.kube/config ./build/e2e -test.v #-test.run=TestTrafficSwitch
```
