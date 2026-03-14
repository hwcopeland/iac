#!/bin/sh

errorExit() {
  echo "*** $*" 1>&2
  exit 1
}

curl -sfk https://localhost:6443/healthz -o /dev/null || errorExit "API server health check failed"
