#!/usr/bin/env bats

# App Mesh Gateway e2e tests

set -o errexit

export REPO_ROOT=$(git rev-parse --show-toplevel)

if [[ "${KUBECONFIG}" == "" ]]; then
  export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"
fi

load ${REPO_ROOT}/test/e2e-lib.sh

mesh=appmesh
name=appmesh-gateway
namespace=appmesh-gateway

function setup() {
  infof "Installing prerequisites"
  applyCRDs
  applyMesh
  waitForMesh $mesh $namespace
}

@test "App Mesh Gateway install" {
  kubectl apply -k ${REPO_ROOT}/kustomize/appmesh-gateway-nodeport
  waitForService $name $namespace
  waitForDeployment $name $namespace
  waitForVirtualNode $name $namespace
}

function teardown() {
  kubectl delete ns $namespace || true
}
