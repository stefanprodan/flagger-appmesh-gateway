#!/usr/bin/env bash

# App Mesh Gateway e2e testing helpers

set -o errexit

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

function applyCRDs() {
  kubectl apply -k github.com/aws/eks-charts/stable/appmesh-controller//crds?ref=master >&3
}

function applyMesh() {
  cat <<EOF | kubectl apply -f -
apiVersion: appmesh.k8s.aws/v1beta1
kind: Mesh
metadata:
  name: appmesh
spec:
  serviceDiscoveryType: dns
EOF
}


