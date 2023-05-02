#!/bin/bash

echo running coverage test
go test -race -covermode=atomic -coverprofile=coverage.out ./...

echo opening coverage html report
go tool cover -html=coverage.out
