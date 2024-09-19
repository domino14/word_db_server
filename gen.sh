#!/usr/bin/env bash
set -e

buf_generate() {
  echo "Starting buf generate"
  (
    cd rpc || exit
    buf generate
  )
}

command -v buf >/dev/null 2>&1 || { echo "buf command not found. Please install buf."; exit 1; }

buf_generate

echo "Done. Thank you for using our generator."