#!/bin/sh

set -x
set -e

env

# must be passed in via Environment
# INFLUX_ORG
# INFLUXDB_URL
ADMIN_INFLUX_TOKEN=${ADMIN_INFLUX_TOKEN:-$DOCKER_INFLUXDB_INIT_ADMIN_TOKEN}

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

trap 'echo "exiting"; for i in /tmp/*.json; do echo "===== $i"; cat $i; done' EXIT

###############################################
# setup influx user for grafana
###############################################

user="grafana"

# save all data in ${CONFIGDIR}/container-info-grafana.txt as persistent store.
# Only recreate if empty
CONFIGDIR=${CONFIGDIR:-/config}
.  ${CONFIGDIR}/container-info-grafana.txt

if [[ -z $INFLUX_TOKEN ]]; then
  INFLUX_TOKEN=$(get_influx_token  $user $INFLUX_ORG)
fi

echo "INFLUX_TOKEN=$INFLUX_TOKEN" > ${CONFIGDIR}/container-info-grafana.txt

###############################################
# configure grafana - get API key
###############################################

if [[ -z $GRAFANA_APIKEY ]]; then
  # create admin api key
  GRAFANA_APIKEY=$(curl \
     -H "Content-Type: application/json" \
     --user admin:admin \
     ${GRAFANA_URL}/api/auth/keys \
     -d '{"name":"AdminAPIKey", "role": "Admin"}' |
     tee -a /tmp/apikeyresp.json | jq -r .key)
fi

echo "GRAFANA_APIKEY=$GRAFANA_APIKEY" >> ${CONFIGDIR}/container-info-grafana.txt

# Authorization header with Bearer token *should* be preferred mechanism
# but doesnt work for unknown reason: --header "Authorization: Bearer ${GRAFANA_APIKEY}"
# use this instead: --user api_key:$GRAFANA_APIKEY

###############################################
# configure grafana - setup influx data source
###############################################

echo "add grafana source"
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
      \"name\":\"InfluxDB\",
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

# TODO


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

exit 0

