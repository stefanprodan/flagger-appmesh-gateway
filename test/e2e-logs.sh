#!/usr/bin/env bash

# App Mesh Gateway deployment status and logs

set -o errexit

export REPO_ROOT=$(git rev-parse --show-toplevel)
export KUBECONFIG="$(kind get kubeconfig-path --name="kind")"


name=appmesh-gateway
namespace=appmesh-gateway

function status() {
  kubectl -n $namespace describe deployment/$name || true
  kubectl -n $namespace get pods || true
}

function logs() {
  kubectl -n $namespace logs deployment/$name || true
}

status
logs

