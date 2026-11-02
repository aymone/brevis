#!/usr/bin/env bash
# The DuckDB oracle for the Iceberg destination: a reader that is not
# iceberg-go reads the table the integration test wrote, and finds the row
# count and the sum the test left for it.
#
# WHY A SECOND READER. iceberg-go reading its own files proves consistency,
# not interoperability; the destination exists so that DuckDB, Spark and
# Trino read the lake, and DuckDB is the one a CI runner can carry. The
# version is PINNED (spike S2, 2026-11-02: 1.4.5, 1.5.6 and 2.0.0 read the
# same table, 2.0.0 without the documented iceberg_metadata bug), and the
# scan goes by the metadata.json path -- no catalog on the reading side.
#
# IT DOES NOT PASS BY SKIPPING. The integration test writes the file this
# reads; no file means the test did not run, and that is a failure here, not
# a warning: a green oracle that read nothing is the exact defect the
# workflow's own comments warn about.
#
#   BREVIS_ICEBERG_TEST_ORACLE=/path/to/oracle.json   written by the test
#   BREVIS_ICEBERG_TEST_S3_ENDPOINT=http://host:port  the S3 the table is on
#   AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION
#   DUCKDB=duckdb                                      the CLI, 2.0.0
set -euo pipefail

oracle="${BREVIS_ICEBERG_TEST_ORACLE:?BREVIS_ICEBERG_TEST_ORACLE names the file the integration test writes}"
endpoint="${BREVIS_ICEBERG_TEST_S3_ENDPOINT:?BREVIS_ICEBERG_TEST_S3_ENDPOINT is the S3 the table is on}"
duckdb="${DUCKDB:-duckdb}"
# DuckDB 2.0 changes its output when AI_AGENT is set in the environment; the
# oracle parses CSV and wants the plain shell.
unset AI_AGENT

if [ ! -f "$oracle" ]; then
  echo "::error::$oracle is missing: TestIntegrationSQLCatalogOnPostgresWithS3 did not write it, so the oracle has nothing to read."
  echo "::error::This step is not allowed to pass by skipping."
  exit 1
fi

read -r meta rows sum < <(python3 -c '
import json, sys
o = json.load(open(sys.argv[1]))
print(o["metadata"], o["rows"], o["sum_quantity"])
' "$oracle")

# DuckDB wants host:port and a USE_SSL flag, not a URL.
host="${endpoint#http://}"; host="${host#https://}"
ssl=false; case "$endpoint" in https://*) ssl=true;; esac

sql="INSTALL iceberg; LOAD iceberg; INSTALL httpfs; LOAD httpfs;
CREATE SECRET lake (TYPE s3, KEY_ID '${AWS_ACCESS_KEY_ID:-brevis}', SECRET '${AWS_SECRET_ACCESS_KEY:-brevis-secret}',
  REGION '${AWS_REGION:-us-east-1}', ENDPOINT '$host', URL_STYLE 'path', USE_SSL $ssl);
SELECT count(*), coalesce(sum(quantity), 0) FROM iceberg_scan('$meta');"

echo "duckdb $("$duckdb" --version) reads $meta"
got=$("$duckdb" -csv -noheader -c "$sql" | tail -n 1)
got_rows="${got%%,*}"; got_sum="${got##*,}"

if [ "$got_rows" != "$rows" ] || [ "$got_sum" != "$sum" ]; then
  echo "::error::DuckDB read $got_rows rows with sum(quantity) $got_sum; the writer left $rows rows with sum $sum"
  exit 1
fi
echo "✅ DuckDB read $got_rows rows, sum(quantity) $got_sum, the same the destination wrote"
