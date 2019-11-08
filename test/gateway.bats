#!/usr/bin/env bats

# App Mesh Gateway e2e tests

set -o errexit

export REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"

load ${REPO_ROOT}/test/e2e-lib.sh

mesh=appmesh
name=flagger-appmesh-gateway
namespace=appmesh-gateway

function setup() {
  applyCRDs
  applyMesh
  waitForMesh $mesh $namespace
}

@test "App Mesh Gateway" {
  # run kustomization for the locally built image
  kubectl apply -k ${REPO_ROOT}/kustomize/nodeport
  kubectl -n $namespace set image deployment/$name controller=test/flagger-appmesh-gateway:latest

  # run install tests
  waitForService $name $namespace
  waitForDeployment $name $namespace
  waitForVirtualNode $name $namespace

  # run discovery tests
  kubectl apply -k ${REPO_ROOT}/kustomize/test
  waitForVirtualService "podinfo.test" "test"
  waitForVirtualNodeBackend $name $namespace "podinfo.test"
}

