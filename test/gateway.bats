#!/usr/bin/env bats

# App Mesh Gateway e2e tests

set -o errexit

export REPO_ROOT=$(git rev-parse --show-toplevel)

if [[ "${KUBECONFIG}" == "" ]]; then
  export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"
fi

name=appmesh-gateway
namespace=appmesh-gateway

function infof() {
    echo -e "\e[32m${1}\e[0m" >&3
}

function errorf() {
    echo -e "\e[31m${1}\e[0m" >&3
    exit 1
}

function waitForDeployment() {
  infof "Waiting for deployment $1"
  retries=10
  count=0
  ok=false
  until $ok; do
    kubectl -n $2 get deployment/$1 && ok=true || ok=false
    sleep 6
    count=$(($count + 1))
    if [[ $count -eq $retries ]]; then
      errorf "No more retries left"
    fi
  done

  kubectl -n $2 rollout status deployment/$1 --timeout=1m >&3
  infof "✔ deployment/$1 test passed"
}

function waitForService() {
  infof "Waiting for service $1"
  retries=10
  count=0
  ok=false
  until $ok; do
    kubectl -n $2 get svc/$1 && ok=true || ok=false
    sleep 6
    count=$(($count + 1))
    if [[ $count -eq $retries ]]; then
      errorf "No more retries left"
    fi
  done
    infof "✔ service/$1 test passed"
}

function waitForVirtualNode() {
  infof "Waiting for virtual node $1"
  retries=10
  count=0
  ok=false
  until $ok; do
    kubectl -n $2 get virtualnode/$1 && ok=true || ok=false
    sleep 6
    count=$(($count + 1))
    if [[ $count -eq $retries ]]; then
      errorf "No more retries left"
    fi
  done
    infof "✔ service/$1 test passed"
}

function setup() {
  infof "Preparing namespace $namespace"
  kubectl create ns $namespace >&3
  kubectl label ns $namespace appmesh.k8s.aws/sidecarInjectorWebhook=enabled >&3
  kubectl apply -k github.com/aws/eks-charts/stable/appmesh-controller//crds?ref=master >&3
}

@test "App Mesh Gateway" {
  kubectl apply -k ${REPO_ROOT}/kustomize/appmesh-gateway-nodeport
  waitForService $name $namespace
  waitForDeployment $name $namespace
  waitForVirtualNode $name $namespace
}

function teardown() {
  kubectl delete ns $namespace || true
}
