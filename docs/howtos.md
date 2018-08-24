# How to's

## Configure port mapping

`StackSets` creates the underlying resource types `Ingress`, `Service` and
`Deployment`. In order to connect these resources at the port level they must
be configured to use the same port number or name. For `StackSets` there are a
number of configuration options possible for setting up this port mapping.

### Configure Ingress backendPort

The simplest option is to configure the optional `backendPort` value under
`spec.ingress` as shown below:

```yaml
apiVersion: zalando.org/v1
kind: StackSet
metadata:
  name: my-app
spec:
  # optional Ingress definition.
  ingress:
    hosts: [my-app.example.org]
    backendPort: ingress # defaults to 'ingress' if not specified
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
              name: ingress
```

This will result in an `Ingress` resource where the `servicePort` value is
`ingress`:

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
          servicePort: ingress
```

And since the `podTemplate` of the `StackSet` also defines a named port `ingress`
for the container:

```yaml
containers:
- name: nginx
  image: nginx
  ports:
  - containerPort: 80
    name: ingress
```

The service created for a `Stack` will get the following generated port
configuration:

```yaml
ports:
- name: ingress
  port: 80
  protocol: TCP
  targetPort: 80
```

Which ensures that there is a connection from `Ingress -> Service -> Pods`.

If you have multiple ports or containers defined in a pod it's important that
exactly one of the ports map to the `backendPort` defined for the ingress
(default value: `ingress`).

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
    backendPort: ingress # defaults to 'ingress' if not specified
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

Here you must make sure that `spec.ingress.backendPort` map to one of the ports
defined for the service and that the service ports map to container ports.
