FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/main

FROM alpine:3.22
RUN adduser -D -H -u 10001 server && mkdir -p /data && chown server /data

COPY --from=build /out/server /usr/local/bin/server

WORKDIR /data
VOLUME /data

USER server
EXPOSE 8080

ENTRYPOINT ["server"]
