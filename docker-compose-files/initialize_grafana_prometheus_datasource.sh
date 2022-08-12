#!/bin/sh

set -x
set -e

env

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
#      \"orgID\":\"${org_id}\",
#\"typeLogoUrl\":\"\",
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

exit 0
