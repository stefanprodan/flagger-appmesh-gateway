# KxDS
[![CI](https://github.com/stefanprodan/kxds/workflows/CI/badge.svg)](https://github.com/stefanprodan/kxds/actions)

KxDS is an Envoy discovery service implementation for Kubernetes.
KxDS runs as a sidecar next to Envoy and configures the proxy to expose Kubernetes services.

### Features

* **Kubernetes Service Discovery** KxDS watches Kubernetes for services with a `http` named port
* **App Mesh Service Discovery** KxDS watches Kubernetes for App Mesh virtual services
* **Envoy Clusters (CDS)** are generated for each Kubernetes service or App Mesh virtual services
* **Envoy Routes (RDS)** are generated for each cluster and configured with timeouts and retry policies
* **Envoy Weighted Clusters** are generated based on Kubernetes service annotations
* **Envoy Listeners (LDS)** KxDS configures Envoy to listen on port `8080`

### Install

API Gateway for Kubernetes

```sh
kubectl apply -k github.com/stefanprodan/kxds//kustomize/gateway
```

API Gateway for App Mesh

```sh
kubectl apply -k github.com/stefanprodan/kxds//kustomize/appmesh-gateway
```

### Annotations

Kubernetes service exposed on an external domain:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: demo
  annotations:
    envoy.gateway.kubernetes.io/expose: "true"
    envoy.gateway.kubernetes.io/timeout: "25s"
    envoy.gateway.kubernetes.io/retries: "5"
    envoy.gateway.kubernetes.io/domain: "frontend.example.com"
spec:
  ports:
    - name: http
      port: 9898
      protocol: TCP
```

Traffic split with weighted destinations:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: backend
  namespace: demo
  annotations:
    envoy.gateway.kubernetes.io/domain: "backend.demo"
    envoy.gateway.kubernetes.io/primary: "backend-primary-demo-9898"
    envoy.gateway.kubernetes.io/canary: "backend-canary-demo-9898"
    envoy.gateway.kubernetes.io/canary-weight: "50"
```

The primary and canary name format is `<service-name>-<namespace>-<port>`.
Note that both Kubernetes services must exist or Envoy will reject the configuration.
