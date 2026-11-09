#!/bin/sh
# Dagster's Postgres storage wants the driver named in the address, where DATABASE_URL has postgres://.
export DAGSTER_PG_URL="postgresql+psycopg2://${DATABASE_URL#*://}"
exec "$@"
