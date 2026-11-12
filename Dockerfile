# Build stage: compile a static binary.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bosho ./cmd/bosho

# Run stage: a small image with only the binary, run as a non-root user.
FROM alpine:3.22
RUN adduser -D -u 10001 bosho
USER bosho
COPY --from=build /out/bosho /usr/local/bin/bosho
EXPOSE 8080
ENTRYPOINT ["bosho"]
