#!/bin/bash
#set -x

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

# Version check taken from this fine answer: https://stackoverflow.com/a/4025065/4427375
# This is used to check if the docker compose version is sufficient.
vercomp () {
    if [[ $1 == $2 ]]
    then
        return 0
    fi
    local IFS=.
    local i ver1=($1) ver2=($2)
    # fill empty fields in ver1 with zeros
    for ((i=${#ver1[@]}; i<${#ver2[@]}; i++))
    do
        ver1[i]=0
    done
    for ((i=0; i<${#ver1[@]}; i++))
    do
        if [[ -z ${ver2[i]} ]]
        then
            # fill empty fields in ver2 with zeros
            ver2[i]=0
        fi
        if ((10#${ver1[i]} > 10#${ver2[i]}))
        then
            return 1
        fi
        if ((10#${ver1[i]} < 10#${ver2[i]}))
        then
            return 2
        fi
    done
    return 0
}

testvercomp () {
    vercomp $1 $2
    case $? in
        0) op='=';;
        1) op='>';;
        2) op='<';;
    esac
    if [[ $op != $3 ]]
    then
        echo "FAIL: Your docker compose version is $1. It must be 2.2.0 or higher!"
        exit
    else
        echo "Pass: Docker compose version is $1."
    fi
}


PROFILE_ARG="--profile core"
BUILD_ARG=
DETACH_ARG="-d"
POST_ACTION=

opts=$(getopt \
  -n $(basename $0) \
  -o h \
  --longoptions "build" \
  --longoptions "detach" \
  --longoptions "nodetach" \
  --longoptions "influx-pump" \
  --longoptions "prometheus-pump" \
  --longoptions "splunk-pump" \
  --longoptions "kafka-pump" \
  --longoptions "elk-pump" \
  --longoptions "timescale-pump" \
  --longoptions "influx-test-db" \
  --longoptions "setup-influx-test-db" \
  --longoptions "setup-prometheus-test-db" \
  --longoptions "prometheus-test-db" \
  --longoptions "elk-test-db" \
  --longoptions "timescale-test-db" \
  --longoptions "grafana" \
  -- "$@")
if [[ $? -ne 0 ]]; then
  opts="-h"
  echo
fi

eval set -- "$opts"
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h)
      echo "Usage:  $(basename $0) (start|stop) [--build] [PROFILE...]"
      echo
      echo "Args:"
      echo "  start      Start up dockerized telemetry services"
      echo "  stop       Stop telemetry services"
      echo "  setup      Setup (influx-test-db) one time for Influx and Grafana"
      echo
      echo "Flags:"
      echo "  --build    (Re-)build the docker containers from source"
      echo
      echo "PROFILES:"
      echo "  Pump sets:  in addition to core services. useful when connecting to standalone databases."
      echo "    --influx-pump"
      echo "    --prometheus-pump"
      echo "    --splunk-pump"
      echo "    --kafka-pump"
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
      echo "  --detach|--nodetach   Either detach (default) from docker compose or stay attached and view debug"
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
      SPLUNK=1     
      ;;
    --kafka-pump)
      PROFILE_ARG="$PROFILE_ARG --profile kafka-pump"
      KAFKA=1     
      ;;
    --elk-pump)
      PROFILE_ARG="$PROFILE_ARG --profile elk-pump"
      ;;
    --timescale-pump)
      PROFILE_ARG="$PROFILE_ARG --profile timescale-pump"
      ;;
    --influx-test-db)
      PROFILE_ARG="$PROFILE_ARG --profile influx-test-db"
      INFLUX=1
      ;;    

    --prometheus-test-db)
      PROFILE_ARG="$PROFILE_ARG --profile prometheus-test-db"
      PROMETHEUS=1
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

testvercomp $(docker-compose --version | cut -d ' ' -f 4 | sed 's/^v//') 2.2.0 '>'
set -e

# re-read env settings so we dont regenerate them unnecessarily
[ -e $topdir/.env ] && . $topdir/.env
export DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=${DOCKER_INFLUXDB_INIT_ADMIN_TOKEN:-$(uuidgen -r)}
export DOCKER_INFLUXDB_INIT_PASSWORD=${DOCKER_INFLUXDB_INIT_PASSWORD:-$(uuidgen -r)}
export MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD:-$(uuidgen -r)}
export MYSQL_PASSWORD=${MYSQL_PASSWORD:-$(uuidgen -r)}
export DOCKER_PROMETHEUS_INIT_ADMIN_TOKEN=${DOCKER_PROMETHEUS_INIT_ADMIN_TOKEN:-$(uuidgen -r)}
export DOCKER_PROMETHEUS_INIT_PASSWORD=${DOCKER_PROMETHEUS_INIT_PASSWORD:-$(uuidgen -r)}


# make container user UID match calling user so that containers dont leave droppings we cant remove
> $topdir/.env
echo "USER_ID=$(id -u)" >> $topdir/.env
echo "GROUP_ID=$(id -g)" >> $topdir/.env

# generate some secrets that should be different across all deployments
echo "DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=${DOCKER_INFLUXDB_INIT_ADMIN_TOKEN}" >> $topdir/.env
echo "DOCKER_INFLUXDB_INIT_PASSWORD=${DOCKER_INFLUXDB_INIT_PASSWORD}" >> $topdir/.env
echo "MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}" >> $topdir/.env
echo "MYSQL_PASSWORD=${MYSQL_PASSWORD}" >> $topdir/.env
echo "DOCKER_PROMETHEUS_INIT_ADMIN_TOKEN=${DOCKER_PROMETHEUS_INIT_ADMIN_TOKEN}" >> $topdir/.env
echo "DOCKER_PROMETHEUS_INIT_PASSWORD=${DOCKER_PROMETHEUS_INIT_PASSWORD}" >> $topdir/.env

# init Splink env variables if not set to avoid warnings from docker-compose for other pumps
#if [ -n $SPLUNK ]; then
  if [ -z $SPLUNK_HEC_URL ]; then
    export SPLUNK_HEC_URL=
  fi
  if [ -z $SPLUNK_HEC_KEY ]; then
    export SPLUNK_HEC_KEY=
  fi
  if [ -z $SPLUNK_HEC_INDEX ]; then
    export SPLUNK_HEC_INDEX=
  fi
#fi

mkdir -p $topdir/.certs
chmod 700 $topdir/.certs
#if [ -n $KAFKA ]; then
  if [ -z $KAFKA_BROKER ]; then
    export KAFKA_BROKER=
  fi
  if [ -z $KAFKA_TOPIC ]; then
    export KAFKA_TOPIC=
  fi
  if [ -z $KAFKA_CACERT ]; then
    export KAFKA_CACERT=
  fi
  if [ -z $KAFKA_CLIENT_CERT ]; then
    export KAFKA_CLIENT_CERT=
  fi
  if [ -z $KAFKA_CLIENT_KEY ]; then
    export KAFKA_CLIENT_KEY=
  fi
  if [ -z $KAFKA_SKIP_VERIFY]; then
    export KAFKA_SKIP_VERIFY=
  fi
#fi

 # remove dependency on setup influx-test-db
touch $topdir/docker-compose-files/container-info-influx-pump.txt 
touch $topdir/docker-compose-files/container-info-grafana.txt
touch $topdir/docker-compose-files/container-info-promgrafana.txt

case $1 in
  rm)
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} rm -f
    ;;

  setup)
      if [[ -z $INFLUX ]] && [[ -z $PROMETHEUS ]]; then
          echo  "Setup only with influx-test-db or prometheus-test-db profile... "
          exit 1
      fi
      # RUN ONLY setup containers
      if [[ -n $INFLUX ]]; then
        PROFILE_ARG="--profile setup-influx-test-db"
        POST_ACTION="influx_setup_finish"
      fi  
      if [[ -n $PROMETHEUS ]]; then
        PROFILE_ARG="--profile setup-prometheus-test-db"
        POST_ACTION="prometheus_setup_finish"
      fi

      export CHK_INFLUX_PROMETHEUS=${POST_ACTION}
      echo "CHK_INFLUX_PROMETHEUS=${CHK_INFLUX_PROMETHEUS}" >> $topdir/.env

      DETACH_ARG="-d"
      BUILD_ARG=
      #eval set -- "start"

      for i in  $topdir/docker-compose-files/container-info-influx-pump.txt $topdir/docker-compose-files/container-info-grafana.txt $topdir/docker-compose-files/container-info-promgrafana.txt
      do
        rm -f $i
        touch $i
      done

      # delete any older setup for grafana
      for container in telemetry-reference-tools-influx telemetry-reference-tools-grafana prometheus
      do
        id=$(docker container ls -a --filter name=$container -q)
        if [[ -z $id ]]; then
          continue
        fi
        echo "Cleaning up old containers for $container: $id"
        echo -n "Stopping: "
        docker container stop $id
        echo -n "Removing: "
        docker container rm -v $id
      done

      volume=$(docker volume ls --filter name=telemetry-reference-tools_influxdb-storage -q)
      if [[ -n $volume ]]; then
        echo -n "Removing volume: $volume"
        docker volume rm $volume
      fi
      docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} up ${BUILD_ARG} ${DETACH_ARG}
      ;;  

  stop)
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} stop
    ;;

  start)  
    if  [[ -n $INFLUX ]] && [[ ! -s docker-compose-files/container-info-influx-pump.txt ]]; then
      echo "Influx must be set up before running. Please run setup --influx-test-db first"
      exit 1
    fi
    echo "prometheus variable is: $PROMETHEUS"
    if  [[ -n $PROMETHEUS ]] && [[ ! -s docker-compose-files/container-info-promgrafana.txt ]]; then
      echo "Prometheus must be set up before running. Please run setup --prometheus-test-db first"
      exit 1
    fi

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

influx_setup_finish() {
  while ! grep INFLUX_TOKEN $topdir/docker-compose-files/container-info-influx-pump.txt;
  do
    echo "Waiting for container setup to finish"
    sleep 1
  done
  echo "Influx pump container setup done. "

  while ! grep GRAFANA_DASHBOARD_CREATED $topdir/docker-compose-files/container-info-grafana.txt;
  do
    echo "Waiting for grafana container setup Influx DATA_SOURCE & DASHBOARD to finish"
    sleep 1
  done

    echo "grafana container setup done for datasource and dashboards. Shutting down."
  
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} stop

#  echo "Removing completed setup containers that are no longer needed"
    docker container rm -v $(docker container ls -a --filter ancestor=idrac-telemetry-reference-tools/setup -q)
}

prometheus_setup_finish() {
  while ! grep GRAFANA_PROM_DASHBOARD_CREATED $topdir/docker-compose-files/container-info-promgrafana.txt;
  do
    echo "Waiting for grafana container setup Prometheus DATA_SOURCE & DASHBOARD to finish"
    sleep 1
  done

    echo "grafana container setup done for datasource and dashboards. Shutting down."
  
    docker-compose --project-directory $topdir -f $scriptdir/docker-compose.yml ${PROFILE_ARG} stop

#  echo "Removing completed setup containers that are no longer needed"    
    docker container rm -v $(docker container ls -a --filter ancestor=idrac-telemetry-reference-tools/setupprometheus -q)
}

$POST_ACTION
