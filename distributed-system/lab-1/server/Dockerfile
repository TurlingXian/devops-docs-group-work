# official image from GOLANG
FROM golang:latest AS base-go

WORKDIR /usr/src/app
COPY . .
EXPOSE 8080
RUN cd advance-server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v

FROM alpine:latest


RUN addgroup -S application && adduser -S application -G application

WORKDIR /app

COPY --from=base-go --chown=application:application /usr/src/app/advance-server/advance-server .
USER application

# ENTRYPOINT [ "/bin/advance-server" ]
ENTRYPOINT [ "/app/advance-server" ]
