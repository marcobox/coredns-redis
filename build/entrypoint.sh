#!/bin/sh

CONF_FILE="/Corefile"

if [ -n "$COREDNS_FORWARD" ]; then
  CONF_FILE="/Corefile.forward"
fi

if [ -n "$1" ]; then
  exec "$@"
fi

exec ./coredns -conf "${CONF_FILE}"
