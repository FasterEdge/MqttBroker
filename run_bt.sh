#!/bin/sh
export HOME=/tmp
export GOCACHE=/tmp/gocache
export GOFLAGS=-mod=mod
cd /repo/MqttBroker
echo "=== go vet ==="
go vet ./... 2>&1
echo "vet_exit=$?"
echo "=== go build -buildvcs=false ==="
go build -buildvcs=false ./... 2>&1
echo "build_exit=$?"
echo "=== go test -buildvcs=false ==="
go test -buildvcs=false ./... 2>&1
echo "test_exit=$?"