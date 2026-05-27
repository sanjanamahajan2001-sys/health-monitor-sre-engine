#!/bin/bash
export PATH=$PATH:/usr/local/go/bin:$(go env GOPATH)/bin
go build -o ./health-monitor ./cmd/health-monitor/
