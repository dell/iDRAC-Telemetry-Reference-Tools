#!/bin/sh

set -x
set -e

env

# must be passed in via Environment
# INFLUXDB_ORG
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

function set_user_passwd() {
  local user_id=$1
  local password=$2

  curl --fail -s --request GET \
    "${INFLUXDB_URL}/api/v2/users/$user_id/password" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data '{"password": "'$password'"}' |
    tee -a /tmp/set_user_password.json
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
          {\"action\": \"read\", \"resource\": {\"type\": \"buckets\"}},
          {\"action\": \"write\", \"resource\": {\"type\": \"buckets\"}}
          ]
        }" | tee -a /tmp/create_token.json
}

get_influx_token() {
  local user=$1
  local user_id=$2
  local org=$3
  local org_id=$(get_org_id $org)

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

user="telemetry"

# save all data in ${CONFIGDIR}/container-info-grafana.txt as persistent store.
# Only recreate if empty
CONFIGDIR=${CONFIGDIR:-/config}
.  ${CONFIGDIR}/container-info-influx-pump.txt

user_id=$(get_user_id $user)

if [[ -z $INFLUX_TOKEN ]]; then
  INFLUX_TOKEN=$(get_influx_token  $user $user_id $INFLUXDB_ORG)
fi

echo "INFLUXDB_USER=$user" > ${CONFIGDIR}/container-info-influx-pump.txt
echo "INFLUX_TOKEN=$INFLUX_TOKEN" >> ${CONFIGDIR}/container-info-influx-pump.txt

###############################################
# set pass for telemetry user
###############################################
if [[ -z $INFLUXDB_PASS ]]; then
  INFLUXDB_PASS=$(uuidgen -r)
  set_user_passwd "$user"  "$INFLUXDB_PASS"
fi
echo "INFLUXDB_PASS=$INFLUXDB_PASS" >> ${CONFIGDIR}/container-info-influx-pump.txt

###############################################
# create telemetry database
###############################################


if [[ -z "$CREATE_DB" ]]; then
  curl -v --request POST \
    "${INFLUXDB_URL}/query" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data-urlencode "q=CREATE DATABASE ${INFLUXDB_DB}" |
    tee /tmp/create_database.json
  INFLUX_CREATE_DB=1
fi
echo "INFLUX_CREATE_DB=$INFLUX_CREATE_DB" >> ${CONFIGDIR}/container-info-influx-pump.txt

if [[ -z "$INFLUX_GRANT_DBd" ]]; then
  curl -v --request POST \
    "${INFLUXDB_URL}/query" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data-urlencode "q=GRANT ALL ON \"${INFLUXDB_DB}\" to \"${user}\"" |
    tee /tmp/grant.json
  INFLUX_GRANT_DB=1
fi
echo "INFLUX_GRANT_DB=$INFLUX_GRANT_DB" >> ${CONFIGDIR}/container-info-influx-pump.txt
