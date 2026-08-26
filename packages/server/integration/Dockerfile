# Integration test image: boots the registry server with its embedded
# SQLite database and drives the npm protocol against it with a real npm
# client (see run.sh). Build from packages/server:
#
#   docker build -f integration/Dockerfile -t gmpm-integration .

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/main

FROM node:26-alpine
RUN apk add --no-cache bash curl jq

COPY --from=build /out/server /usr/local/bin/server
COPY integration/ /opt/integration

WORKDIR /opt/integration
ENTRYPOINT ["bash", "run.sh"]
