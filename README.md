# Kubernetes StackSet Controller
[![Build Status](https://travis-ci.org/zalando-incubator/stackset-controller.svg?branch=master)](https://travis-ci.org/zalando-incubator/stackset-controller)
[![Coverage Status](https://coveralls.io/repos/github/zalando-incubator/stackset-controller/badge.svg?branch=master)](https://coveralls.io/github/zalando-incubator/stackset-controller?branch=master)

The Kubernetes StackSet Controller is a concept (along with an
implementation) for easing and automating application life cycle for
certain types of applications running on Kubernetes.

It is not meant to be a generic solution for all types of applications but it's
explicitly focusing on "Web Applications", that is, application which receive
HTTP traffic and are continuously deployed with new versions which should
receive traffic either instantly or gradually fading traffic from one version
of the application to the next one. Think Blue/Green deployments as one
example.

By default Kubernetes offers the
[Deployment](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)
resource type which, combined with a
[Service](https://kubernetes.io/docs/concepts/services-networking/service/),
can provide some level of application life cycle in the form of rolling updates.
While rolling updates are a powerful concept, there are some limitations for
certain use cases:

* Switching traffic in a Blue/Green style is not possible with rolling
  updates.
* Splitting traffic between versions of the application can only be done by
  scaling the number of Pods. E.g. if you want to give 1% of traffic to a new
  version, you need at least 100 Pods.
* Impossible to run smoke tests against a new version of the application before
  it gets traffic.

To work around these limitations I propose a different type of resource called
an `StackSet` which has the concept of `Stacks`.

The `StackSet` is a declarative way of describing the application stack as a
whole, and the `Stacks` describe individual versions of the
application. The `StackSet` also allows defining a "global" load balancer
spanning all stacks of the stackset which makes it possible to switch
traffic to different stacks at the load balancer (for example Ingress) level.


```
                                 +-----------------------+
                                 |                       |
                                 |     Load Balancer     |
                                 | (for example Ingress) |
                                 |                       |
                                 +--+--------+--------+--+
                                    | 0%     | 20%    | 80%
                      +-------------+        |        +------------+
                      |                      |                     |
            +---------v---------+  +---------v---------+  +--------v----------+
            |                   |  |                   |  |                   |
            |       Stack       |  |       Stack       |  |      Stack        |
            |     Version 1     |  |     Version 2     |  |     Version 3     |
            |                   |  |                   |  |                   |
            +-------------------+  +-------------------+  +-------------------+
```

The `StackSet` and `Stack` resources are implemented as
[CRDs][crd].  A `StackSet` looks like this:

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  # optional Ingress definition.
  ingress:
    hosts: [my-app.example.org, alt.name.org]
    backendPort: 80
  # optional desired traffic weights defined by stack
  traffic:
  - stackName: mystack-v1
    weight: 80
  - stackName: mystack-v2
    weight: 20
  # optional percentage of required Replicas ready to allow traffic switch
  # if none specified, defaults to 100
  minReadyPercent: 90
  stackLifecycle:
    scaledownTTLSeconds: 300
    limit: 5 # maximum number of scaled down stacks to keep.
             # If there are more than `limit` stacks, the oldest stacks which are scaled down
             # will be deleted.
  stackTemplate:
    spec:
      version: v1 # version of the Stack.
      replicas: 3
      # optional autoscaler definition (will create an HPA for the stack).
      autoscaler:
        minReplicas: 3
        maxReplicas: 10
        metrics:
        - type: CPU
          averageUtilization: 50
      # full Pod template.
      podTemplate:
        spec:
          containers:
          - name: skipper
            image: ghcr.io/zalando/skipper:latest
            args:
            - skipper
            - -inline-routes
            - '* -> inlineContent("OK") -> <shunt>'
            - -address=:80
            ports:
            - containerPort: 80
              name: ingress
            resources:
              limits:
                cpu: 10m
                memory: 50Mi
              requests:
                cpu: 10m
                memory: 50Mi
```

The above `StackSet` would generate a `Stack` that looks like this:

```yaml
apiVersion: zalando.org/v1
kind: Stack
metadata:
  name: my-app-v1
  labels:
    stackset: my-app
    stackset-version: v1
spec:
  replicas: 3
  autoscaler:
    minReplicas: 3
    maxReplicas: 10
    metrics:
    - type: CPU
      averageUtilization: 50
  podTemplate:
    spec:
      containers:
        image: ghcr.io/zalando/skipper:latest
        args:
        - skipper
        - -inline-routes
        - '* -> inlineContent("OK") -> <shunt>'
        - -address=:80
        ports:
        - containerPort: 80
          name: ingress
        resources:
          limits:
            cpu: 10m
            memory: 50Mi
          requests:
            cpu: 10m
            memory: 50Mi
```

For each `Stack` a `Service` and `Deployment` resource will be created
automatically with the right labels. The service will also be attached to the
"global" Ingress if the stack is configured to get traffic. An optional
`autoscaler` resource can also be created per stack for
horizontally scaling the deployment.

For the most part the `Stacks` will be dynamically managed by the
system and the users don't have to touch them. You can think of this similar to
the relationship between `Deployments` and `ReplicaSets`.

If the `Stack` is deleted the related resources like `Service` and
`Deployment` will be automatically cleaned up.

The `stackLifecycle` let's you configure two settings to change the cleanup
behavior for the `StackSet`:

* `scaleDownTTLSeconds` defines for how many seconds a stack should not receive
  traffic before it's scaled down.
* `limit` defines the total number of stacks to keep. That is, if you have a
  `limit` of `5` and currently have `6` stacks for the `StackSet` then it will
  clean up the oldest stack which is **NOT** getting traffic. The `limit` is
  not enforced if it would mean deleting a stack with traffic. E.g. if you set
  a `limit` of `1` and have two stacks with `50%` then none of them would be
  deleted. However, if you switch to `100%` traffic for one of the stacks then
  the other will be deleted after it has not received traffic for
  `scaleDownTTLSeconds`.

## Features

* Automatically create new Stacks when the `StackSet` is updated with a new
  version in the `stackTemplate`.
* Do traffic switching between Stacks at the Ingress layer, if you
  have the ingress definition in the spec. Ingress
  resources are automatically updated when new stacks are created. (This
  require that your ingress controller implements the annotation
  `zalando.org/backend-weights: {"my-app-1": 80, "my-app-2": 20}`, for
  example use [skipper](https://github.com/zalando/skipper) for
  Ingress) or read the information from stackset `status.traffic`.
* Safely switch traffic to scaled down stacks. If a stack is scaled down, it
  will be scaled up automatically before traffic is directed to it.
* Dynamically provision Ingresses per stack, with per stack host names. I.e.
    `my-app.example.org`, `my-app-v1.example.org`, `my-app-v2.example.org`.
* Automatically scale down stacks when they don't get traffic for a specified
  period.
* Automatically delete stacks that have been scaled down and are not getting
  any traffic for longer time.
* Automatically clean up all dependent resources when a `StackSet` or
    `Stack` resource is deleted. This includes `Service`,
    `Deployment`, `Ingress` and optionally `HorizontalPodAutoscaler`.
* Command line utility (`traffic`) for showing and switching traffic between
  stacks.
* You can opt-out of the global `Ingress` creation with
  `externalIngress:` spec, such that external controllers can manage
  the Ingress or CRD creation, that will configure the routing into
  the cluster.
* You can use skipper's
  [RouteGroups](https://opensource.zalando.com/skipper/kubernetes/routegroups)
  to configure more complex routing rules.

## Docs

* [How To's](/docs/howtos.md)

### Kubernetes Compatibility

The StackSet controller works with Kubernetes `>=v1.23`.

## How it works

The controller watches for `StackSet` resources and creates `Stack` resources
whenever the version is updated in the `StackSet` `stackTemplate`. For each
`StackSet` it will create an optional "main" `Ingress` resource and keep it up
to date when new `Stacks` are created for the `StackSet`. For each `Stack` it
will create a `Deployment`, a `Service` and optionally an
`HorizontalPodAutoscaler` for the `Deployment`. These resources are all owned
by the `Stack` and will be cleaned up if the stack is deleted.

## Setup

Use an existing cluster or create a test cluster with [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)

```bash
kind create cluster --name testcluster001
```

The `stackset-controller` can be run as a deployment in the cluster.
See [deployment.yaml](/docs/deployment.yaml).

The controller depends on the [StackSet](/docs/stackset_crd.yaml) and
[Stack](/docs/stack_crd.yaml)
[CRDs][crd].
You must install these into your cluster before running the controller:

```bash
$ kubectl apply -f docs/stackset_crd.yaml -f docs/stack_crd.yaml
```

After the CRDs are installed the controller can be deployed:

*please adjust the controller version and cluster-domain to your environment*

```bash
$ kubectl apply -f docs/rbac.yaml -f docs/deployment.yaml
```

### Custom configuration

## controller-id

There are cases where it might be desirable to run multiple instances of the
stackset-controller in the same cluster, e.g. for development.

To prevent the controllers from fighting over the same `StackSet` resources
they can be configured with the flag `--controller-id=<some-id>` which
indicates that the controller should only manage the `StackSets` which has an
annotation `stackset-controller.zalando.org/controller=<some-id>` defined.
If the controller-id is not configured, the controller will manage all
`StackSets` which does not have the annotation defined.

## Quick intro

Once you have deployed the controller you can create your first `StackSet`
resource:

```bash
$ kubectl apply -f docs/stackset.yaml
stackset.zalando.org/my-app created
```

This will create the stackset in the cluster:

```bash
$ kubectl get stacksets
NAME          CREATED AT
my-app        21s
```

And soon after you will see the first `Stack` of the `my-app`
stackset:

```bash
$ kubectl get stacks
NAME                  CREATED AT
my-app-v1             30s
```

It will also create `Ingress`, `Service`, `Deployment` and
`HorizontalPodAutoscaler` resources:

```bash
$ kubectl get ingress,service,deployment.apps,hpa -l stackset=my-app
NAME                           HOSTS                   ADDRESS                                  PORTS     AGE
ingress.extensions/my-app      my-app.example.org      kube-ing-lb-3es9a....elb.amazonaws.com   80        7m
ingress.extensions/my-app-v1   my-app-v1.example.org   kube-ing-lb-3es9a....elb.amazonaws.com   80        7m

NAME                TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)  AGE
service/my-app-v1   ClusterIP   10.3.204.136   <none>        80/TCP   7m

NAME                        DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/my-app-v1   1         1         1            1           7m

NAME                                            REFERENCE              TARGETS         MINPODS   MAXPODS   REPLICAS   AGE
horizontalpodautoscaler.autoscaling/my-app-v1   Deployment/my-app-v1   <unknown>/50%   3         10        0          20s
```

Imagine you want to roll out a new version of your stackset. You can do this
by changing the `StackSet` resource. E.g. by changing the version:

```bash
$ kubectl patch stackset my-app --type='json' -p='[{"op": "replace", "path": "/spec/stackTemplate/spec/version", "value": "v2"}]'
stackset.zalando.org/my-app patched
```

Soon after, we will see a new stack:

```bash
$ kubectl get stacks -l stackset=my-app
NAME        CREATED AT
my-app-v1   14m
my-app-v2   46s
```

And using the `traffic` tool we can see how the traffic is distributed (see
below for how to build the tool):

```bash
./build/traffic my-app
STACK          TRAFFIC WEIGHT
my-app-v1      100.0%
my-app-v2      0.0%
```

If we want to switch 100% traffic to the new stack we can do it like this:

```bash
# traffic <stackset> <stack> <traffic>
./build/traffic my-app my-app-v2 100
STACK          TRAFFIC WEIGHT
my-app-v1      0.0%
my-app-v2      100.0%
```

Since the `my-app-v1` stack is no longer getting traffic it will be scaled down
after some time and eventually deleted.

If you want to delete it manually, you can simply do:

```bash
$ kubectl delete appstack my-app-v1
stacksetstack.zalando.org "my-app-v1" deleted
```

And all the related resources will be gone shortly after:

```bash
$ kubectl get ingress,service,deployment.apps,hpa -l stackset=my-app,stackset-version=v1
No resources found.
```

## Building

This project uses [Go modules](https://github.com/golang/go/wiki/Modules) as
introduced in Go 1.11 therefore you need Go >=1.11 installed in order to build.
If using Go 1.11 you also need to [activate Module
support](https://github.com/golang/go/wiki/Modules#installing-and-activating-module-support).

Assuming Go has been setup with module support it can be built simply by running:

```sh
$ export GO111MODULE=on # needed if the project is checked out in your $GOPATH.
$ make
```

Note that the Go client interface for talking to the custom `StackSet` and
`Stack` CRD is generated code living in `pkg/client/` and
`pkg/apis/zalando.org/v1/zz_generated_deepcopy.go`. If you make changes to
`pkg/apis/*` then you must run `make clean && make` to regenerate the code.

To understand how this works see the upstream
[example](https://github.com/kubernetes/apiextensions-apiserver/tree/master/examples/client-go)
for generating client interface code for CRDs.

[crd]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/

## Upgrade

### <= v1.0.0 to >= v1.1.0

Clients that write the desired traffic switching value have to move
from ingress annotation `zalando.org/stack-traffic-weights: '{"mystack-v1":80, "mystack-v2": 20}'`
to stackset `spec.traffic`:

```yaml
spec:
  traffic:
  - stackName: mystack-v1
    weight: 80
  - stackName: mystack-v2
    weight: 20
```
