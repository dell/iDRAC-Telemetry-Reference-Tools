#!/bin/bash

set -x
set -e

export PGPASSFILE=/tmp/.pgpass
touch $PGPASSFILE
chmod 600 $PGPASSFILE
echo "*:*:*:$POSTGRES_USER:$POSTGRES_PASS" > $PGPASSFILE

sleep 10

echo "create telemetry database if it does not exist"
echo "
  SELECT
    'CREATE DATABASE $TIMESCALE_DB WITH OWNER $POSTGRES_USER;' |
    'GRANT ALL PRIVILEGES ON DATABASE $TIMESCALE_DB TO $POSTGRES_USER;'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = $TIMESCALE_DB)\gexec" |
  psql -h $POSTGRES_HOST --port $POSTGRES_PORT --username postgres <<EOF
