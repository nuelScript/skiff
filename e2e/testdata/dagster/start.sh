#!/bin/sh
# Dagster's Postgres driver wants postgresql:// where DATABASE_URL has postgres://.
export DAGSTER_PG_URL="postgresql://${DATABASE_URL#*://}"
exec "$@"
