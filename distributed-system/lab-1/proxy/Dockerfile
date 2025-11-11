FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o proxy .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/proxy .
EXPOSE 9999

CMD ["./proxy", "9999"]