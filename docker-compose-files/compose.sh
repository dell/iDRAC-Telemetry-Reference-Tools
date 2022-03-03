#!/bin/bash 
scriptdir=$(cd $(dirname $0); pwd)
topdir=$(cd $scriptdir/../; pwd)
cd $topdir

if [[ $(id -u) = 0 ]]; then
  echo "Please do not run $(basename $0) as root, it is designed to be run as a normal user account that has docker permissions."
  exit 1
fi

if [[ $# -eq 0 ]] ; then
    echo 'No arguments provided. Run with -h for help.'
    exit 0
fi

PROFILE_ARG=
BUILD_ARG=
DETACH_ARG="-d"

opts=$(getopt \
  -n $(basename $0) \
  -o h \
  --longoptions "build" \
  --longoptions "detach" \
  --longoptions "nodetach" \
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
      echo "Start options:"
      echo "  --detach|--nodetach   Either detach from docker compose or stay attached and view debug"
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
    --detach)
      DETACH_ARG="-d"
      ;;
    --nodetach)
      DETACH_ARG=""
      ;;
    --)
      shift
      break
      ;;
  esac
  shift
done

# re-read env settings so we dont regenerate them unnecessarily
[ -e $topdir/.env ] && . $topdir/.env
export DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=${DOCKER_INFLUXDB_INIT_ADMIN_TOKEN:-$(uuidgen -r)}
export DOCKER_INFLUXDB_INIT_PASSWORD=${DOCKER_INFLUXDB_INIT_PASSWORD:-$(uuidgen -r)}

# make container user UID match calling user so that containers dont leave droppings we cant remove
> $topdir/.env
echo "USER_ID=$(id -u)" >> $topdir/.env
echo "GROUP_ID=$(id -g)" >> $topdir/.env

# generate some secrets that should be different across all deployments
echo "DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=${DOCKER_INFLUXDB_INIT_ADMIN_TOKEN}" >> $topdir/.env
echo "DOCKER_INFLUXDB_INIT_PASSWORD=${DOCKER_INFLUXDB_INIT_PASSWORD}" >> $topdir/.env

touch $topdir/docker-compose-files/container-info-influx-pump.txt
touch $topdir/docker-compose-files/container-info-grafana.txt

case $1 in
  rm)
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} rm -f
    ;;

  stop)
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} stop
    ;;

  start)
    echo "Set up environment file in $topdir/.env"
    echo "To run manually, run the following command line:"
    echo "docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} up ${BUILD_ARG} ${DETACH_ARG}"
    echo
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} up ${BUILD_ARG} ${DETACH_ARG}
    ;;

  *)
    echo "Specify 'start' or 'stop'"
    exit 1
    ;;
esac
