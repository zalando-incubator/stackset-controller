### Running the End-To-End Tests

The following environment variables should be set:

1. `E2E_NAMESPACE` is the namespace where the tests should be run.
2. `CLUSTER_DOMAIN` is the DNS domain managed in which ingresses should be created
3. `CONTROLLER_ID` is set so that all stacks are only managed by the controller being currently tested.
4. `KUBECONFIG` with the path to the kubeconfig file

To run the tests run the command:

```
go test -parallel 32 github.com/zalando-incubator/stackset-controller/cmd/e2e
```
