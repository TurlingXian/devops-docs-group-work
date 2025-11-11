FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o http_server .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/http_server .

COPY --from=builder /app/webroot ./webroot

EXPOSE 8080

CMD ["./http_server", "8080"]