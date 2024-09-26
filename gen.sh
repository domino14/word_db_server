#!/usr/bin/env bash
set -e

buf_generate() {
  echo "Starting buf generate"
  buf generate
}

sqlc_generate() {
  echo "Starting sqlc generate"
  sqlc generate
}

command -v buf >/dev/null 2>&1 || { echo "buf command not found. Please install buf."; exit 1; }
command -v sqlc >/dev/null 2>&1 || { echo "sqlc command not found. Please install sqlc."; exit 1; }

buf_generate
sqlc_generate

echo "Done. Thank you for using our generator."