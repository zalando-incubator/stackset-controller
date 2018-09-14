# How to's

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
