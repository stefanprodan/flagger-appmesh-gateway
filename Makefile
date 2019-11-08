TAG?=latest
VERSION?=$(shell grep 'const VERSION' cmd/flagger-appmesh-gateway/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)
NAME:=flagger-appmesh-gateway
DOCKER_REPOSITORY:=weaveworks
DOCKER_IMAGE_NAME:=$(DOCKER_REPOSITORY)/$(NAME)

build:
	go build -o bin/flagger-appmesh-gateway cmd/flagger-appmesh-gateway/*.go

test:
	go test -v -race ./...

go-fmt:
	gofmt -l pkg/* | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi;

run:
	go run cmd/flagger-appmesh-gateway/*.go --kubeconfig=$$HOME/.kube/config -v=4 \
	--gateway-mesh=appmesh --gateway-name=gateway --gateway-namespace=flagger-appmesh-gateway

envoy:
	envoy -c envoy.yaml -l info

build-container:
	docker build -t $(DOCKER_IMAGE_NAME):v$(VERSION) .

push-container: build-container
	docker push $(DOCKER_IMAGE_NAME):v$(VERSION)

version-set:
	@next="$(TAG)" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" cmd/flagger-appmesh-gateway/main.go && \
	sed -i '' "s/flagger-appmesh-gateway:v$$current/flagger-appmesh-gateway:v$$next/g" kustomize/base/gateway/deployment.yaml && \
	echo "Version $$next set in code and kustomization"
