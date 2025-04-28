#!/usr/bin/env bash

SCRIPT_DIR=$(dirname $(readlink -e ${BASH_SOURCE[0]}))

function create() {
  if [[ ! $(docker network ls | grep prometheus) ]]; then
    docker network create prometheus
  fi

  docker run \
    -d \
    --name prometheus \
    --network prometheus \
    --add-host host.docker.internal:host-gateway \
    -p 9090:9090 \
    -v $SCRIPT_DIR/config.yaml:/etc/prometheus/prometheus.yml \
    prom/prometheus:v3.2.1
}

function clean() {
  docker container stop prometheus ||:
  docker container rm prometheus ||:
  docker network rm prometheus ||:
}

if [[ $# -ne 1 ]]; then
  echo "specify 'create' or 'clean'"
  exit 1
fi

case $1 in
  create)
    create
    ;;
  clean)
    clean
    ;;
  *)
    echo "$1 is not a valid operation, specify 'create' or 'clean'"
    exit 1
    ;;
esac
