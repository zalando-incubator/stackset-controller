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
          - name: nginx
            image: nginx
            ports:
            - containerPort: 80
```

This will result in an `Ingress` resource where the `servicePort` value is
`80`:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: my-app
spec:
  rules:
  - host: my-app.example.org
    http:
      paths:
      - backend:
          serviceName: my-app-v1
          servicePort: 80
```

And since the `podTemplate` of the `StackSet` also defines a containerPort `80`
for the container:

```yaml
containers:
- name: nginx
  image: nginx
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
case it's posible to define the service ports on the `StackSet`.

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
          - name: nginx
            image: nginx-with-metrics
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
with an equivalent spec. Currently, the autoscaler can be used to specify scaling based on 4 metrics:

1. `CPU`
2. `Memory`
3. `AmazonSQS`
4. `PodJSON`
5. `Ingress`

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
  - type: Memory
    averageUtilization: 80
```

Here the stackset would be scaled based on the length of the Amazon SQS Queue size so that there are no more
than 30 items in the queue per pod. Also the autoscaler tries to keep the CPU usage below 80% in the pods by
scaling. If multiple metrics are specified then the HPA calculates the number of pods required per metrics
and uses the higest recommendation.

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
    ingress:
      name: my-app
    average: 30
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
2. Before the stack gets any traffic it will caculate a prescale value `n` of
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
It will still prescale to the sum of stacks getting traffic within each step.
This means that it might overscale for some minutes before the HPA kicks in and
scales back down to the needed resources. Reliability is favoured over cost in
the prescale logic.

## Traffic Switch resources controlled by External Controllers

External controllers could create routes based on multiple Ingress,
[RouteGroup](https://github.com/zalando/skipper/blob/feature/routegroup-crd/dataclients/kubernetes/crd.md),
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
