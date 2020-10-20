# syntax = docker/dockerfile:experimental@sha256:de85b2f3a3e8a2f7fe48e8e84a65f6fdd5cd5183afa6412fff9caa6871649c44

# golang:1.15.2
FROM golang@sha256:da7ff43658854148b401f24075c0aa390e3b52187ab67cab0043f2b15e754a68 AS stage1

ENV GOCACHE=/root/.cache/go/gocache
ENV GOMODCACHE=/root/.cache/go/gomodcache
ENV GOPATH=

COPY . .

RUN --mount=type=cache,target=/root/.cache/go go build github.com/play-with-go/preguide/cmd/preguide

# busybox:1.32.0
FROM busybox@sha256:30d1412c0f45be67d38b99179866868b1f09fd9013cbacf22813926aee428cf7

RUN mkdir /runbin

COPY --from=stage1 /go/preguide /runbin
