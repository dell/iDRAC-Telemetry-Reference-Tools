#!/bin/bash 
scriptdir=$(cd $(dirname $0); pwd)
topdir=$(cd $scriptdir/../../..; pwd)
cd $topdir

PROFILE_ARG=
BUILD_ARG=

opts=$(getopt \
  -n $(basename $0) \
  -o h \
  --longoptions "build" \
  --longoptions "influx-pump" \
  --longoptions "prometheus-pump" \
  --longoptions "splunk-pump" \
  --longoptions "elk-pump" \
  --longoptions "timescale-pump" \
  --longoptions "influx-test-db" \
  --longoptions "prometheus-test-db" \
  --longoptions "elk-test-db" \
  --longoptions "timescale-test-db" \
  --longoptions "grafana" \
  -- "$@")
if [[ $? -ne 0 ]]; then
  opts="-h"
  echo
fi

set -e

eval set -- "$opts"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h)
      echo "Usage:  $(basename $0) (start|stop) [--build] [PROFILE...]"
      echo
      echo "Args:"
      echo "  start      Start up dockerized telemetry services"
      echo "  stop       Stop telemetry services"
      echo
      echo "Flags:"
      echo "  --build    (Re-)build the docker containers from source"
      echo
      echo "PROFILES:"
      echo "  Pump sets:  in addition to core services. useful when connecting to standalone databases."
      echo "    --influx-pump"
      echo "    --prometheus-pump"
      echo "    --splunk-pump"
      echo "    --elk-pump"
      echo "    --timescale-pump"
      echo
      echo "  demonstration test databases:"
      echo "    --influx-test-db"
      echo "    --prometheus-test-db"
      echo "    --elk-test-db"
      echo "    --timescale-test-db"
      echo
      exit 0
      ;;
    --build)
      BUILD_ARG="--build --remove-orphans"
      ;;
    --influx-pump)
      PROFILE_ARG="$PROFILE_ARG --profile influx-pump"
      ;;
    --prometheus-pump)
      PROFILE_ARG="$PROFILE_ARG --profile prometheus-pump"
      ;;
    --splunk-pump)
      PROFILE_ARG="$PROFILE_ARG --profile splunk-pump"
      ;;
    --elk-pump)
      PROFILE_ARG="$PROFILE_ARG --profile elk-pump"
      ;;
    --timescale-pump)
      PROFILE_ARG="$PROFILE_ARG --profile timescale-pump"
      ;;
    --influx-test-db)
      PROFILE_ARG="$PROFILE_ARG --profile influx-test-db"
      ;;
    --prometheus-test-db)
      PROFILE_ARG="$PROFILE_ARG --profile prometheus-test-db"
      ;;
    --elk-test-db)
      PROFILE_ARG="$PROFILE_ARG --profile elk-test-db"
      ;;
    --timescale-test-db)
      PROFILE_ARG="$PROFILE_ARG --profile timescale-test-db"
      ;;
    --grafana)
      PROFILE_ARG="$PROFILE_ARG --profile grafana"
      ;;
    --)
      shift
      break
      ;;
  esac
  shift
done

# make container user UID match calling user so that containers dont leave droppings we cant remove
echo "USER_ID=$(id -u)" > $topdir/.env
echo "GROUP_ID=$(id -g)" >> $topdir/.env

case $1 in
  stop)
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml rm -f
    ;;

  start)
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} up ${BUILD_ARG}
    ;;

  *)
    echo "Specify 'start' or 'stop'"
    exit 1
    ;;
esac
