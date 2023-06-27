# How to's

* [Configure port mapping](#configure-port-mapping)
* [Specifying Horizontal Pod Autoscaler](#specifying-horizontal-pod-autoscaler)
* [Enable stack prescaling](#enable-stack-prescaling)

## Configure port mapping

`StackSets` creates `Stacks` which in turn creates the underlying resource
types `Ingress`, `Service` and `Deployment`. In order to connect these
resources at the port level they must be configured to use the same port number
or name. For `StackSets` there are a number of configuration options possible
for setting up this port mapping.

### Configure Ingress backendPort

The `backendPort` value under `spec.ingress` must be defined as shown below:

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  # optional Ingress definition.
  ingress:
    hosts: [my-app.example.org]
    backendPort: 80
  stackTemplate:
    spec:
      version: v1
      replicas: 3
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
```

This will result in an `Ingress` resource where the `service.port.number` value is
`80`:

```yaml
apiVersion: networking/v1
kind: Ingress
metadata:
  name: my-app
spec:
  rules:
  - host: my-app.example.org
    http:
      paths:
      - backend:
          service:
            name: my-app-v1
            port:
              number: 80
```

And since the `podTemplate` of the `StackSet` also defines a containerPort `80`
for the container:

```yaml
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
```

The service created for a `Stack` will get the following generated port
configuration:

```yaml
ports:
- name: port-0
  port: 80
  protocol: TCP
  targetPort: 80
```

Which ensures that there is a connection from `Ingress -> Service -> Pods`.

If you have multiple ports or containers defined in a pod it's important that
exactly one of the ports map to the `backendPort` defined for the ingress i.e.
one port of the containers must have the same name or port number as the
`backendPort`.

### Configure custom service ports

In some cases you want to expose multiple ports in your service. For this use
case it's possible to define the service ports on the `StackSet`.

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  # optional Ingress definition.
  ingress:
    hosts: [my-app.example.org]
    backendPort: 80
  stackTemplate:
    spec:
      version: v1
      replicas: 3
      # define custom ports for the service
      service:
        ports:
        - name: ingress
          port: 80
          protocol: TCP
          targetPort: 8080
        - name: metrics
          port: 79
          protocol: TCP
          targetPort: 7979
      podTemplate:
        spec:
          containers:
          - name: skipper
            image: ghcr.io/zalando/skipper:latest
            args:
            - skipper
            - -inline-routes
            - 'Path("/metrics") -> inlineContent("app_amazing_metric 42") -> <shunt>'
            - -inline-routes
            - '* -> inlineContent("OK") -> <shunt>'
            - -address=:80
            ports:
            - containerPort: 8080
              name: ingress
            - containerPort: 7979
              name: metrics
```

Here you must make sure that the value used for `spec.ingress.backendPort` also
maps to one of the ports in the `Service`, either by name or port number.
Additionally the service ports should map to corresponding container ports also
by name or port number.

### Specifying Horizontal Pod Autoscaler

A [Horizontal Pod Autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
can be attached to the the deployment created by the stackset.
HPAs can be used to scale the number of pods based on metrics from different sources.
Specifying an HPA for a deployment allows the stack to scale up during periods of higher
traffic and then scale back down during off-peak hours to save costs.

HPAs can be specified in 2 different ways for stacksets. The first is to use the `horizontalPodAutoscaler`
field which is similar in syntax to the original Horizontal Pod Autoscaler. The second way is to use
the `autoscaler` field. This is then resolved by the _stackset-controller_ which generates an HPA
with an equivalent spec. Currently, the autoscaler can be used to
specify scaling based on the following metrics:

1. `CPU`
2. `Memory`
3. `AmazonSQS`
4. `PodJSON`
5. `Ingress`
6. `RouteGroup`
7. `RequestsPerSecond`
8. `ZMON`
9. `ScalingSchedule`
10. `ClusterScalingSchedule`

_Note:_ Based on the metrics type specified you may need to also deploy the [kube-metrics-adapter](https://github.com/zalando-incubator/kube-metrics-adapter)
in your cluster.

Following is an example using the `autoscaler` field to generate an HPA with
CPU metrics, Memory metrics and external metrics based on AmazonSQS queue size.

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 3
  metrics:
  - type: AmazonSQS
    queue:
      name: foo
      region: eu-west-1
    average: 30
  - type: CPU
    averageUtilization: 80
    # optional: scale based on metrics from a single named container as
    # opposed to the average of all containers in a pod.
    container: "app"
  - type: Memory
    averageUtilization: 80
    # optional: scale based on metrics from a single named container as
    # opposed to the average of all containers in a pod.
    container: "app"
```

Here the stackset would be scaled based on the length of the Amazon SQS Queue size so that there are no more
than 30 items in the queue per pod. Also the autoscaler tries to keep the CPU usage below 80% in the pods by
scaling. If multiple metrics are specified then the HPA calculates the number of pods required per metrics
and uses the highest recommendation.

JSON metrics exposed by the pods are also supported. Here's an example where the pods expose metrics in
JSON format on the `/metrics` endpoint on port 9090. The key for the metrics should be specified as well.

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 3
  metrics:
  - type: PodJson
    endpoint:
      port: 9090
      path: /metrics
      key: '$.http_server.rps'
    average: 1k
```

If Skipper is used for [ingress](https://opensource.zalando.com/skipper/kubernetes/ingress-usage/) in
the cluster then scaling can also be done based on the requests received by the stack. The following autoscaler
metric specifies that the number of requests per pod should not be more than 30.

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 3
  metrics:
  - type: Ingress
    average: 30
```

If using `RouteGroup` instead of `Ingress`, then the following config is the
equivalent:

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 3
  metrics:
  - type: RouteGroup
    average: 30
```

If the operator does not wish to reference a `Ingress` or `RouteGroup` resource for some reason like RouteGroups are 
being generated by `FabricGateway`, and thus its hard to actually find which resource to reference using other metrics,
a external metric like `RequestsPerSecond` might be pretty handy, see the example:

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 3
  metrics:
  - type: RequestsPerSecond
    average: 30
    hostnames: 
        - 'example.com'
        - 'foo.bar.baz'
    weight: 100
```
Note that `weight` field is not mandatory, in case it is not specified it will be assumed the value is 100%, and `hostnames` accepts multiple hostnames the result is the sum of average RPS of each hostname will be compared to the target specified by `average` field.

If ZMON based metrics are supported you can enable scaling based on ZMON checks
as shown in the following metric configuration:

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 3
  metrics:
  - type: ZMON
    zmon:
      checkID: "1234"
      key: "custom.value"
      duration: "5m"
      aggregators:
      - avg
      tags:
        application: "my-app"
    average: 30
```

Metrics to scale based on time are also supported. It relies on the
[`ScalingSchedule`
collectors](https://github.com/zalando-incubator/kube-metrics-adapter#scalingschedule-collectors).
The following is an example of metrics configuration, for both
`ScalingSchedule` and `ClusterScalingSchedule` resources:

```yaml
autoscaler:
  minReplicas: 1
  maxReplicas: 30
  metrics:
  - type: ClusterScalingSchedule
    # The average value per pod of the returned metric
    average: 1000
    clusterScalingSchedule:
      # The name of the deployed ClusterScalingSchedule object
      name: "cluster-wide-scheduling-event"
  - type: ScalingSchedule
    # The average value per pod of the returned metric
    average: 10
    scalingSchedule:
      # The name of the deployed ScalingSchedule object
      name: "namespaced-scheduling-event"
```

## Enable stack prescaling

The stackset-controller has `alpha` support for prescaling stacks before
directing traffic to them. That is, if you deploy your stacks with Horizontal
Pod Autoscaling (HPA) enabled then you might have the current stack scaled to
20 pods while a new stack is initially deployed with only 3 pods. In this case
you want to make sure that the new stack is scaled to 20 pods before it gets
any traffic, otherwise it might die under the high unexpected load and the HPA
would not be able to react and scale up fast enough.

To enable prescaling support, you simply need to add the
`alpha.stackset-controller.zalando.org/prescale-stacks` annotation to your
`StackSet` resource:

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
  annotations:
    alpha.stackset-controller.zalando.org/prescale-stacks: "yes"
    # alpha.stackset-controller.zalando.org/reset-hpa-min-replicas-delay: 20m # optional
spec:
...
```

### Prescaling logic

The pre scaling works as follows:

1. User directs/increases traffic to a stack.
2. Before the stack gets any traffic it will calculate a prescale value `n` of
   replicas based on the sum of all stacks currently getting traffic.
3. The HPA of the stack will get the `MinReplicas` value set equal to the
   prescale value calculated in 2.
4. Once the stack has `n` ready pods then traffic will be switched to it.
5. After the traffic has been switched and has been running for 10 min. then
   the HPA `MinReplicas` will be reset back to what is configured in the
   stackset allowing the HPA to scale down in case the load decreases for the
   service.

The default delay for resetting the `MinReplicas` of the HPA is 10 min. You can
configure the time by setting the
`alpha.stackset-controller.zalando.org/reset-hpa-min-replicas-delay` annotation
on the stackset.

**Note**: Even if you switch traffic gradually like `10%...20%..50%..80%..100%`
It will still prescale based on the **_sum of all stacks_** getting traffic within each step.
This means that it might overscale for some minutes before the HPA kicks in and
scales back down to the needed resources. Reliability is favoured over cost in
the prescale logic.

### Overscaling Example

To explain further, the amount of this overscale is decided as follows. Imagine
a case with `minReplicas: 20` and `maxReplicas: 40` and traffic is switched in
steps as `1%`, `25%`, `50%` and `100%`. Additionally, imagine the existing
stack has 20 replicas running.

1. Upon switching of `1%` of the traffic to the new stack, the amount of
prescale should be 1 % of 20 (sum of all stacks sizes receiving traffic currently).
However, since this is less than `minReplicas`, the scale will be up to
`minReplicas` and will be 20.
2. Before switching to `25%` of the traffic, the stack is increased in size by
25% of 40 = 10 (20 from old stack + 20 from the new), so the new stack is
scaled to 20 + 10 = 30 replicas before the switch.
3. Before switching to `50%`, same follows, and the prescale is now done as
50% of 50 = 25 (30 replicas of new stack + 20 replicas of the old), and so
the new stack is to be scaled to 55 replicas, however, since `maxReplicas` is
set to 40, stack size will be set to `40`.
4. Similarly, when `100%` of the traffic is to be switched, the size of
`maxReplicas` will be enforced.

## Traffic Switch resources controlled by External Controllers

External controllers could create routes based on multiple Ingress,
[FabricGateway](https://github.com/zalando-incubator/fabric-gateway/blob/master/deploy/operator/apply/01_FabricGatewayCRD.yaml)
[RouteGroup](https://opensource.zalando.com/skipper/kubernetes/routegroup-crd/)
[SMI](https://smi-spec.io/),
[Istio](https://istio.io/docs/tasks/traffic-management/request-routing/)
or other CRDs.

Known controllers, that support this:
- https://github.com/zalando-incubator/fabric-gateway
- `<add-yours-here>`

### Configure ExternalIngress backendPort

Users need to provide the `backendPort` value under `spec.externalIngress` as shown below:

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  externalIngress:
    backendPort: 8080
```

### Integrating an External Controller

Clients set the traffic switch states in `spec.traffic`.
External controllers should read actual traffic switch status from
`status.traffic`. The `status.traffic` has all information to link to
the right Kubernetes service.

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  traffic:
  - stackName: my-app-v1
    weight: 80
  - stackName: my-app-v2
    weight: 20
status:
  observedStackVersion: v2
  stacks: 2
  stacksWithTraffic: 1
  traffic:
  - stackName: my-app-v1
    serviceName: my-app-v1
    servicePort: 8080
    weight: 100
  - stackName: my-app-v2
    serviceName: my-app-v2
    servicePort: 8080
    weight: 0
```

## Using RouteGroups

[RouteGroups](https://opensource.zalando.com/skipper/kubernetes/routegroups)
is a skipper specific CRD and are a more powerful routing
configuration than ingress. For example you want to redirect `/login`,
if the cookie "my-login-cookie" is not set, to your Open ID Connect
provider or something like this?  Here is how you can do that:

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  routegroup:
    additionalBackends:
    - name: theShunt
      type: shunt
    backendPort: 9090
    hosts:
    - "www.example.org"
    routes:
    - pathSubtree: "/"
    # route with more predicates has more weight, than the redirected route
    - path: "/login"
      predicates:
      - Cookie("my-login-cookie")
    - path: "/login"
      # backends within a route overwrites
      backends:
      - backendName: theShunt
      filters:
      - redirectTo(308, "https://login.example.com")
  stackTemplate:
    spec:
      version: v1
      replicas: 3
      podTemplate:
        spec:
          containers:
          - name: skipper
            image: ghcr.io/zalando/skipper:latest
            args:
            - skipper
            - -inline-routes
            - 'r0: * -> inlineContent("OK") -> <shunt>; r1: Path("/login") -> inlineContent("login") -> <shunt>;'
            - -address=:9090
            ports:
            - containerPort: 9090
```
