#!/usr/bin/env bash

go get ./pkg/tdfs
go build -o tdfs ./pkg/tdfs/tdfs.go

if [ "$(uname)" == "Darwin" ]; then
    mv tdfs ~/bin/tdfs     
elif [ "$(expr substr $(uname -s) 1 5)" == "Linux" ]; then
    sudo mv tdfs /usr/local/bin/tdfs
fi