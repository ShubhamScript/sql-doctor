# Multi-stage Dockerfile for SQL Doctor
FROM golang:1.26-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/sql-doctor ./cmd/sql-doctor

FROM alpine:latest

WORKDIR /root/

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/sql-doctor /usr/local/bin/sql-doctor

ENTRYPOINT ["sql-doctor"]
CMD ["--help"]
