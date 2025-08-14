#!/bin/bash

set -euo pipefail
cd "$(dirname "$0")/.."

go test $(go list ./... | grep -v /examples/) \
    -coverprofile=tmp/cover.out \
    -covermode=atomic || exit 1

go tool cover -func=tmp/cover.out | sed -n '1,220p'
