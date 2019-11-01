TAG?=latest
VERSION?=$(shell grep 'VERSION' cmd/appmesh-gateway/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)
NAME:=appmesh-gateway
DOCKER_REPOSITORY:=stefanprodan
DOCKER_IMAGE_NAME:=$(DOCKER_REPOSITORY)/$(NAME)

build:
	go build -o bin/appmesh-gateway cmd/appmesh-gateway/*.go

test:
	go test -v -race ./...

go-fmt:
	gofmt -l pkg/* | grep ".*\.go"; if [ "$$?" = "0" ]; then exit 1; fi;

run:
	go run cmd/appmesh-gateway/*.go --kubeconfig=$$HOME/.kube/config -v=4 \
	--gateway-mesh=appmesh --gateway-name=gateway --gateway-namespace=appmesh-gateway

envoy:
	envoy -c envoy.yaml -l info

build-container:
	docker build -t $(DOCKER_IMAGE_NAME):v$(VERSION) .

push-container: build-container
	docker push $(DOCKER_IMAGE_NAME):v$(VERSION)

version-set:
	@next="$(TAG)" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" cmd/appmesh-gateway/main.go && \
	sed -i '' "s/appmesh-gateway:v$$current/appmesh-gateway:v$$next/g" kustomize/base/appmesh-gateway/deployment.yaml && \
	echo "Version $$next set in code and kustomization"
