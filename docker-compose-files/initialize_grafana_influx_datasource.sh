#!/bin/sh
# this file is for both Influx and Prometheus to use with Grafana
set -x
set -e

env

#sleep 3

CONFIGDIR=${CONFIGDIR:-/config}

INFLUX_OR_PRM=${INFLUX_OR_PRM:-$CHK_INFLUX_PROMETHEUS}

if [ "$INFLUX_OR_PRM" != "$INFLUX_OR_PROMETHEUS" ]; then
  ######################################### Prometheus block #########################################
  # must be passed in via Environment
  # INFLUX_ORG
  # PROMETHEUS_URL
  DASHBOARDDIRS=${DASHBOARDDIRS:-/dashboardstgt}

  # Step 1: create grafana user in the influx

  function get_apikey() {

    curl -H "Content-Type: application/json" \
      --user admin:admin \
      ${GRAFANA_URL}/api/auth/keys \
      -d '{"name":"AdminAPIKey", "role": "Admin"}' | tee -a /tmp/apikeyresp.json | jq -r ".key"
  }

  trap 'echo "exiting"; for i in /tmp/*.json; do echo "===== $i"; cat $i; done' EXIT

  ###############################################
  # setup influx user for grafana
  ###############################################

  user="grafana"

  # save all data in ${CONFIGDIR}/container-info-promgrafana.txt as persistent store.
  # Only recreate if empty
  CONFIGDIR=${CONFIGDIR:-/config}
  .  ${CONFIGDIR}/container-info-promgrafana.txt

  # wait until influx is ready
  while ! curl --fail -v "${PROMETHEUS_URL}/graph" \
      --header 'Content-type: application/json'
  do
      sleep 1
  done

  ###############################################
  # configure grafana - get API key
  ###############################################

  # wait until grafana is ready
  while ! curl --fail -v "${GRAFANA_URL}/api/auth/keys" \
      --user admin:admin \
      --header 'Content-type: application/json'
  do
      sleep 1
  done

  #this 5 sec temporarily put to avoid the GRAFANA_APIKEY getting null, need to remove once that is fixed.
  sleep 5

  GRAFANA_APIKEY=${GRAFANA_APIKEY:-$(get_apikey)}

  echo "GRAFANA_APIKEY=$GRAFANA_APIKEY" >> ${CONFIGDIR}/container-info-promgrafana.txt

  # Authorization header with Bearer token *should* be preferred mechanism
  # but doesnt work for unknown reason: --header "Authorization: Bearer ${GRAFANA_APIKEY}"
  # use this instead: --user api_key:$GRAFANA_APIKEY

  ###############################################
  # configure grafana - setup prometheus data source
  ###############################################

  datasourceprom="PrometheusDataSource"
  
  if [[ -z $GRAFANA_PROM_DATA_SOURCE_CONNECTED ]]; then
    curl --fail -s --request POST \
      "${GRAFANA_URL}/api/datasources" \
      --user api_key:$GRAFANA_APIKEY  \
      --header 'Content-type: application/json' \
      --data "{
        \"id\":\"${1}\",
        \"orgID\":\"${1}\",
        \"url\":\"${PROMETHEUS_URL}\",
        \"user\":\"${user}\",
        \"version\":1,
        \"readOnly\":false,
        \"name\":\"${datasourceprom}\",
        \"type\":\"prometheus\",
        \"access\":\"proxy\",
        \"password\":\"\",
        \"basicAuth\":false,
        \"basicAuthUser\":\"\",
        \"basicAuthPassword\":\"\",
        \"withCredentials\":false,
        \"isDefault\":false
      }" | tee -a /tmp/grafana-source.json
    GRAFANA_PROM_DATA_SOURCE_CONNECTED=1
  fi

  echo "GRAFANA_PROM_DATA_SOURCE_CONNECTED=$GRAFANA_PROM_DATA_SOURCE_CONNECTED" >> ${CONFIGDIR}/container-info-promgrafana.txt

  ###############################################
  # configure grafana - add dashboards
  ###############################################

  # get the uid
  curl --fail -s --request GET \
      -H "Content-Type: application/json" \
      --user api_key:$GRAFANA_APIKEY  \
      ${GRAFANA_URL}/api/datasources/name/${datasourceprom} | tee -a /tmp/uuidprom.json

  GRAFANA_PROM_UID=`cat /tmp/uuidprom.json | jq -r .uid`
  #echo "GRAFANA_PROM_UID=$GRAFANA_PROM_UID" >> ${CONFIGDIR}/container-info-promgrafana.txt
  cntr=$((0))
  # add the dashboards
  for template in ${DASHBOARDDIRS}/*Promtemp.json; do

    sed -e "s/##UID##/$GRAFANA_PROM_UID/g" $template > /tmp/$(basename $template -promtemplate.json).json
    sed -i "s/##DATASRC##/${datasourceprom}/g" /tmp/$(basename $template -promtemplate.json).json

    curl --request POST \
          "${GRAFANA_URL}/api/dashboards/db" \
          --user api_key:$GRAFANA_APIKEY  \
          --header 'Content-type: application/json' \
          --data @/tmp/$(basename $template -promtemplate.json).json
    cntr=$((cntr+1))
  done

  echo "cntr is:${cntr}"

  GRAFANA_PROM_DASHBOARD_CREATED=1
  echo "GRAFANA_PROM_DASHBOARD_CREATED=$GRAFANA_PROM_DASHBOARD_CREATED" >> ${CONFIGDIR}/container-info-promgrafana.txt
else
  ######################################### INFLUX block #########################################
  # must be passed in via Environment
  # INFLUX_ORG
  # INFLUXDB_URL
  ADMIN_INFLUX_TOKEN=${ADMIN_INFLUX_TOKEN:-$DOCKER_INFLUXDB_INIT_ADMIN_TOKEN}
  DASHBOARDDIRS=${DASHBOARDDIRS:-/dashboardstgt}

  # Step 1: create grafana user in the influx

  function create_user() {
    local user=$1
    curl --fail -s --request POST \
      "${INFLUXDB_URL}/api/v2/users/" \
      --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
      --header 'Content-type: application/json' \
      --data "{\"name\": \"$1\"}" | tee -a /tmp/create_user.json
  }

  function get_user_id() {
    local user=$1

    curl --fail -s --request GET \
      "${INFLUXDB_URL}/api/v2/users?name=$user" \
      --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
      --header 'Content-type: application/json' |
      tee -a /tmp/get_user_id.json |
      jq -r ".users[0].id"
  }

  function get_org_id() {
    local org=$1

    curl --fail -s --request GET \
      "${INFLUXDB_URL}/api/v2/orgs?name=$org" \
      --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
      --header 'Content-type: application/json' |
      tee -a /tmp/get_org_id.json |
      jq -r ".orgs[0].id"
  }

  function create_token() {
    local user=$1
    local user_id=$2
    local org_id=$3

    curl --fail -s --request POST \
      "${INFLUXDB_URL}/api/v2/authorizations" \
      --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
      --header 'Content-type: application/json' \
      --data "{
          \"orgID\": \"${org_id}\",
            \"userID\": \"${user_id}\",
            \"description\": \"${user}\",
            \"permissions\": [
            {\"action\": \"read\", \"resource\": {\"type\": \"buckets\"}}
            ]
          }" | tee -a /tmp/create_token.json
  }

  get_influx_token() {
    local user=$1
    local org=$2
    local org_id=$(get_org_id $org)
    local user_id=$(get_user_id $user)

    if [[ -z $user_id ]]; then
      create_user $user >/dev/null
    fi
    user_id=$(get_user_id $user)
    create_token $user $user_id $org_id | jq -r .token
  }

  function get_apikey() {

    curl -H "Content-Type: application/json" \
      --user admin:admin \
      ${GRAFANA_URL}/api/auth/keys \
      -d '{"name":"AdminAPIKey", "role": "Admin"}' | tee -a /tmp/apikeyresp.json | jq -r ".key"
  }

  trap 'echo "exiting"; for i in /tmp/*.json; do echo "===== $i"; cat $i; done' EXIT

  ###############################################
  # setup influx user for grafana
  ###############################################

  user="grafana"

  # save all data in ${CONFIGDIR}/container-info-grafana.txt as persistent store.
  # Only recreate if empty

  .  ${CONFIGDIR}/container-info-grafana.txt

  # wait until influx is ready
  while ! curl --fail -v "${INFLUXDB_URL}/api/v2" \
      --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
      --header 'Content-type: application/json'
  do
      sleep 1
  done

  if [[ -z $INFLUX_TOKEN ]]; then
    INFLUX_TOKEN=$(get_influx_token  $user $INFLUX_ORG)
  fi

  echo "INFLUX_TOKEN=$INFLUX_TOKEN" > ${CONFIGDIR}/container-info-grafana.txt

  ###############################################
  # configure grafana - get API key
  ###############################################

  # wait until grafana is ready
  while ! curl --fail -v "${GRAFANA_URL}/api/auth/keys" \
      --user admin:admin \
      --header 'Content-type: application/json'
  do
      sleep 1
  done

  #this 5 sec temporarily put to avoid the GRAFANA_APIKEY getting null, need to remove once that is fixed.
  sleep 5

  GRAFANA_APIKEY=${GRAFANA_APIKEY:-$(get_apikey)}

  echo "GRAFANA_APIKEY=$GRAFANA_APIKEY" >> ${CONFIGDIR}/container-info-grafana.txt

  # Authorization header with Bearer token *should* be preferred mechanism
  # but doesnt work for unknown reason: --header "Authorization: Bearer ${GRAFANA_APIKEY}"
  # use this instead: --user api_key:$GRAFANA_APIKEY

  ###############################################
  # configure grafana - setup influx data source
  ###############################################

  datasource="InfluxDBDataSource"

  if [[ -z $GRAFANA_DATA_SOURCE_CONNECTED ]]; then
    curl --fail -s --request POST \
      "${GRAFANA_URL}/api/datasources" \
      --user api_key:$GRAFANA_APIKEY  \
      --header 'Content-type: application/json' \
      --data "{
        \"orgID\":\"${org_id}\",
        \"url\":\"${INFLUXDB_URL}\",
        \"database\":\"${INFLUXDB_DB}\",
        \"secureJsonData\":{
          \"token\": \"${INFLUX_TOKEN}\"
        },
        \"jsonData\":{
          \"defaultBucket\":\"${INFLUX_BUCKET}\",
          \"httpMode\":\"POST\",
          \"organization\":\"${INFLUX_ORG}\",
          \"version\":\"Flux\"
        },
        \"user\":\"${user}\",
        \"version\":2,
        \"readOnly\":false,
        \"name\":\"${datasource}\",
        \"type\":\"influxdb\",
        \"typeLogoUrl\":\"\",
        \"access\":\"proxy\",
        \"password\":\"\",
        \"basicAuth\":false,
        \"basicAuthUser\":\"\",
        \"basicAuthPassword\":\"\",
        \"withCredentials\":false,
        \"isDefault\":false
      }" | tee -a /tmp/grafana-source.json
    GRAFANA_DATA_SOURCE_CONNECTED=1
  fi

  echo "GRAFANA_DATA_SOURCE_CONNECTED=$GRAFANA_DATA_SOURCE_CONNECTED" >> ${CONFIGDIR}/container-info-grafana.txt

  ###############################################
  # configure grafana - add dashboards
  ###############################################

  # get the uid
  curl --fail -s --request GET \
      -H "Content-Type: application/json" \
      --user api_key:$GRAFANA_APIKEY  \
      ${GRAFANA_URL}/api/datasources/name/${datasource} | tee -a /tmp/uuid.json

  GRAFANA_UID=`cat /tmp/uuid.json | jq -r .uid`

  # add the dashboards
  for template in ${DASHBOARDDIRS}/*template.json; do

    sed -e "s/##TOKEN##/$INFLUX_TOKEN/g" $template > /tmp/$(basename $template -template.json).json
    sed -i "s/##UID##/$GRAFANA_UID/g" /tmp/$(basename $template -template.json).json
    sed -i "s/##DATASRC##/${datasource}/g" /tmp/$(basename $template -template.json).json
    sed -i "s/##BUCKET##/${INFLUX_BUCKET}/g" /tmp/$(basename $template -template.json).json

    curl --request POST \
          "${GRAFANA_URL}/api/dashboards/db" \
          --user api_key:$GRAFANA_APIKEY  \
          --header 'Content-type: application/json' \
          --data @/tmp/$(basename $template -template.json).json
  done

  GRAFANA_DASHBOARD_CREATED=1
  echo "GRAFANA_DASHBOARD_CREATED=$GRAFANA_DASHBOARD_CREATED" >> ${CONFIGDIR}/container-info-grafana.txt
fi

exit 0
### random grafana queries here for reference. remove after this is all debugged.

# add new org
curl -X POST -H "Content-Type: application/json" -d '{"name":"'"$INFLUX_ORG"'"}' --user admin:admin ${GRAFANA_URL}/api/orgs
# {"message":"Organization created","orgId":6}

# add current user to that org as admin
curl -X POST -H "Content-Type: application/json" -d '{"loginOrEmail":"admin", "role": "Admin"}' --user admin:admin ${GRAFANA_URL}/api/orgs/${grafana_org_id}/users

# switch to using new org
curl -X POST --user admin:admin ${GRAFANA_URL}/api/user/using/$grafana_org_id

# get api key for user
curl -X POST -H "Content-Type: application/json" --user admin:admin -d '{"name":"apikeycurl", "role": "Admin"}' ${GRAFANA_URL}/api/auth/keys
# {"name":"apikeycurl","key":"eyJrIjoiR0ZXZmt1UFc0OEpIOGN5RWdUalBJTllUTk83VlhtVGwiLCJuIjoiYXBpa2V5Y3VybCIsImlkIjo2fQ=="}

#below commented code can be removed after triaging the 5 sec sleep before getting the grafana_apikey
#if [[ -z $GRAFANA_APIKEY ]]; then
  # create admin api key
  #GRAFANA_APIKEY=$(curl \
  #   -H "Content-Type: application/json" \
  #   --user admin:admin \
  #   ${GRAFANA_URL}/api/auth/keys \
  #   -d '{"name":"AdminAPIKey", "role": "Admin"}' | tee -a /tmp/apikeyresp.json | jq -r .key)

 # curl -H "Content-Type: application/json" \
 #    --user admin:admin \
 #    ${GRAFANA_URL}/api/auth/keys \
 #    -d '{"name":"AdminAPIKey", "role": "Admin"}' | tee -a /tmp/apikeyresp.json

 # GRAFANA_APIKEY=`cat /tmp/apikeyresp.json | jq -r .key`
#fi

exit 0

