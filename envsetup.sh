#!/bin/bash

st_build() {
	go build -o sndtool ./cmd/
}

st_format() {
	gofmt -w .
}
