#!/usr/bin/env bash

# Build and load container image in Kubernetes Kind

set -o errexit

function main() {
  docker build -t test/flagger-appmesh-gateway:latest .
  kind load docker-image test/flagger-appmesh-gateway:latest
}

main
