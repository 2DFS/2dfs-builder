#!/usr/bin/env bash

go get ./internal
go build -o tdfs ./internal/tdfs.go