#!/bin/sh

set -o pipefail
set -x

env

# must be passed in via Environment
# INFLUX_ORG
# INFLUXDB_URL
# others...

ADMIN_INFLUX_TOKEN=${ADMIN_INFLUX_TOKEN:-$DOCKER_INFLUXDB_INIT_ADMIN_TOKEN}

# Step 1: create grafana user in the influx

function create_user() {
  local user=$1
  curl --fail -s \
    "${INFLUXDB_URL}/api/v2/users/" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data "{\"name\": \"$1\"}" | tee -a /tmp/create_user.json
}

function add_user_to_org() {
  local org_id=$1
  local user=$2
  local user_id=$3

  curl --fail -s \
    "${INFLUXDB_URL}/api/v2/orgs/${org_id}/members" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data "{\"id\": \"${user_id}\", \"name\": \"$1\"}" | tee -a /tmp/create_user.json
}


function get_user_id() {
  local user=$1

  curl --fail -s --request GET \
    "${INFLUXDB_URL}/api/v2/users?name=${user}" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' |
    tee -a /tmp/get_user_id.json |
    jq -r ".users[0].id"
}

function get_org_id() {
  local org=$1

  curl --fail -s --request GET \
    "${INFLUXDB_URL}/api/v2/orgs?name=${org}" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' |
    tee -a /tmp/get_org_id.json |
    jq -r ".orgs[0].id"
}

function get_bucket_id() {
  local bucket=$1

  curl --fail -s --request GET \
    "${INFLUXDB_URL}/api/v2/buckets?name=${bucket}" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' |
    tee -a /tmp/get_bucket_id.json |
    jq -r ".buckets[0].id"
}

function set_user_passwd() {
  local user_id=$1
  local password=$2

  curl --fail -s --request GET \
    "${INFLUXDB_URL}/api/v2/users/${user_id}/password" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data '{"password": "'$password'"}' |
    tee -a /tmp/set_user_password.json
}

function create_token() {
  local user=$1
  local user_id=$2
  local org_id=$3
  local bucket=$4

  curl --fail -s \
    "${INFLUXDB_URL}/api/v2/authorizations" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json' \
    --data "{
        \"orgID\": \"${org_id}\",
        \"userID\": \"${user_id}\",
        \"description\": \"${user}\",
        \"permissions\": [
          {\"action\": \"read\", \"resource\": {\"type\": \"authorizations\"}},
          {\"action\": \"read\", \"resource\": {\"type\": \"buckets\"}},
          {\"action\": \"write\", \"resource\": {\"type\": \"buckets\", \"name\": \"${bucket}\"}}
        ]
      }" | tee -a /tmp/create_token.json
}

trap 'echo "exiting"; set +x; for i in /tmp/*.json; do echo -e "\n===== $i"; cat $i; done' EXIT

###############################################
# setup influx user for grafana
###############################################

set -x
echo "Start influx configuration"

# save all data in ${CONFIGFILE} as persistent store
# Only recreate if empty
CONFIGDIR=${CONFIGDIR:-/config}
CONFIGFILE=${CONFIGDIR}/container-info-influx-pump.txt
[ -e ${CONFIGFILE} ] && .  ${CONFIGFILE}

INFLUXDB_USER="telemetry"

# wait until influx is ready
while ! curl --fail -v "${INFLUXDB_URL}/api/v2" \
    --header "Authorization: Token ${ADMIN_INFLUX_TOKEN}" \
    --header 'Content-type: application/json'
do
    sleep 1
done

org_id=${org_id:-$(get_org_id $INFLUX_ORG)}

user_id=${user_id:-$(get_user_id $INFLUXDB_USER)}

if [[ -z ${user_id} ]]; then
  user_id=$(create_user $INFLUXDB_USER | jq -r .id )
  add_user_to_org "${org_id}" "${INFLUXDB_USER}" "${user_id}"
fi

###############################################
# set pass for telemetry user
###############################################
INFLUXDB_PASS=${INFLUXDB_PASS:-$(uuidgen -r)}
set_user_passwd "$INFLUXDB_USER"  "$INFLUXDB_PASS"


###############################################
# setup variables for bucket/bucket_id and token
###############################################
INFLUX_BUCKET=${INFLUX_BUCKET:-${INFLUX_ORG}-bucket}
INFLUX_BUCKET_ID=${INFLUX_BUCKET_ID:-$(get_bucket_id $INFLUX_BUCKET)}
INFLUX_TOKEN=${INFLUX_TOKEN:-$(create_token  "$INFLUXDB_USER" "${user_id}" "${org_id}" "${INFLUX_BUCKET}" | jq -r .token)}

echo "INFLUXDB_USER=$INFLUXDB_USER" > ${CONFIGFILE}
echo "org_id=${org_id}"  >> ${CONFIGFILE}
echo "user_id=${user_id}"  >> ${CONFIGFILE}
echo "INFLUXDB_PASS=$INFLUXDB_PASS" >> ${CONFIGFILE}
echo "INFLUX_BUCKET=${INFLUX_BUCKET}" >> ${CONFIGFILE}
echo "INFLUX_BUCKET_ID=${INFLUX_BUCKET_ID}" >> ${CONFIGFILE}
echo "INFLUX_CREATE_DBRPS=${INFLUX_CREATE_DBRPS}" >> ${CONFIGFILE}
echo "INFLUX_TOKEN=$INFLUX_TOKEN" >> ${CONFIGFILE}

exit 0
