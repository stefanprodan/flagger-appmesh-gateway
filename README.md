# appmesh-gateway
[![CI](https://github.com/stefanprodan/appmesh-gateway/workflows/CI/badge.svg)](https://github.com/stefanprodan/appmesh-gateway/actions)
[![report](https://goreportcard.com/badge/github.com/stefanprodan/appmesh-gateway)](https://goreportcard.com/report/github.com/stefanprodan/appmesh-gateway)

App Mesh Gateway is an edge load balancer that exposes applications outside the mesh.

The gateway is composed of:
* [Envoy](https://www.envoyproxy.io/) proxy
* Envoy data plane API (CDS/RDS/LDS)
* Kubernetes controller 

An App Mesh virtual service can be exposed outside the mesh by annotating the object with:

```yaml
apiVersion: appmesh.k8s.aws/v1beta1
kind: VirtualService
metadata:
  name: frontend.test
  annotations:
    gateway.appmesh.k8s.aws/expose: "true"
    gateway.appmesh.k8s.aws/domain: "frontend.example.com"
```

If you want to expose the service inside the Kubernetes cluster you can omit the domain annotation.
By default the gateway exposes a virtual service by its name,
a service can be accessed by setting the host HTTP header e.g.:
```sh
curl -H 'Host: frontend.test' http://<gateway-host>/
```

The gateway registers/de-registers virtual services automatically as they come and go in the cluster.

### Install

Install the API Gateway as NLB in `appmesh-gateway` namespace:

```sh
kubectl apply -k github.com/stefanprodan/appmesh-gateway//kustomize/appmesh-gateway
```

Deploy podinfo in the `test` namespace:

```sh
kubectl -n test apply -k github.com/stefanprodan/appmesh-gateway//kustomize/test
```

Port forward to the gateway:

```sh
kubectl -n appmesh-gateway port-forward svc/appmesh-gateway 8080:80
```

Access the podinfo API by setting the host header to `podinfo.test`:

```sh
curl -vH 'Host: podinfo.test' localhost:8080
```

Access podinfo on its custom domain:

```sh
curl -vH 'Host: podinfo.internal' localhost:8080
```
