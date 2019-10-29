# KxDS

KxDS is an Envoy discovery service implementation for Kubernetes.
KxDS runs as a sidecar next to Envoy and configures the proxy to expose Kubernetes services.

### Features

* **Service Discovery** KxDS watches Kubernetes for ClusterIP services with a `http` named port
* **Envoy Clusters (CDS)** are generated for each Kubernetes service in the form `<name>-<namespace>-<http-port>`
* **Envoy Routes (RDS)** are generated for each cluster and mapped to the `<name>.<namespace>` domain
* **Envoy Listeners (LDS)** KxDS configures Envoy to listen on port `8080` and sets up retry policies for each route

### Install

```sh
kubectl apply -k github.com/stefanprodan/kxds//kustomize/gateway
```

### Annotations

```yaml
apiVersion: v1
kind: Service
metadata:
  annotations:
    envoy.gateway.kubernetes.io/expose: "true"
    envoy.gateway.kubernetes.io/timeout: "25s"
    envoy.gateway.kubernetes.io/retries: "5"
    envoy.gateway.kubernetes.io/domain: "app.internal"
```
