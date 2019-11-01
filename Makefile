TAG?=latest
VERSION?=$(shell grep 'VERSION' cmd/kxds/main.go | awk '{ print $$4 }' | tr -d '"' | head -n1)
NAME:=kxds
DOCKER_REPOSITORY:=stefanprodan
DOCKER_IMAGE_NAME:=$(DOCKER_REPOSITORY)/$(NAME)

build:
	go build -o bin/kxds cmd/kxds/*.go

test:
	go test -v -race ./...

run:
	go run cmd/kxds/*.go kubernetes --kubeconfig=$$HOME/.kube/config \
		--port-name=http

appmesh:
	go run cmd/kxds/*.go appmesh --kubeconfig=$$HOME/.kube/config \
		--gateway-mesh=appmesh --gateway-name=gateway --gateway-namespace=appmesh-gateway

envoy:
	envoy -c envoy.yaml -l info

build-container:
	docker build -t $(DOCKER_IMAGE_NAME):$(VERSION) .

push-container: build-container
	docker push $(DOCKER_IMAGE_NAME):$(VERSION)

version-set:
	@next="$(TAG)" && \
	current="$(VERSION)" && \
	sed -i '' "s/$$current/$$next/g" cmd/kxds/main.go && \
	sed -i '' "s/kxds:v$$current/kxds:v$$next/g" kustomize/base/gateway/deployment.yaml && \
	echo "Version $$next set in code and kustomization"
