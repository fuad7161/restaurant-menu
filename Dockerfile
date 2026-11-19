# syntax=docker/dockerfile:1
# Build stage: compile a static binary.
# The cache mounts keep Go's module downloads and compiled packages between builds, so after the
# first build only the changed code is compiled again (seconds instead of minutes).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bosho ./cmd/bosho

# Run stage: a small image with only the binary, run as a non-root user.
FROM alpine:3.22
RUN adduser -D -u 10001 bosho
USER bosho
COPY --from=build /out/bosho /usr/local/bin/bosho
EXPOSE 8081
ENTRYPOINT ["bosho"]
