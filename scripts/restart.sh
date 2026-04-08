#!/bin/bash
# Restart prd2wiki server
cd /srv/prd2wiki
make build 2>&1

# Kill any existing prd2wiki server (not the mcp sidecar)
PID=$(ss -tlnp | grep ':8082' | grep -oP 'pid=\K[0-9]+')
if [ -n "$PID" ]; then
    kill "$PID" 2>/dev/null
    sleep 2
fi

./bin/prd2wiki > /tmp/prd2wiki-server.log 2>&1 &
sleep 2
if curl -s http://localhost:8082/api/projects/default/pages > /dev/null 2>&1; then
    echo "prd2wiki running on :8082"
else
    echo "FAILED — check /tmp/prd2wiki-server.log"
    cat /tmp/prd2wiki-server.log
fi
