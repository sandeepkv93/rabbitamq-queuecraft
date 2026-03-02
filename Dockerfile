# syntax=docker/dockerfile:1.7
FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/server /app/server

ENV HTTP_ADDR=:8080 \
    APP_MODE=all \
    AMQP_URL=amqp://guest:guest@rabbitmq:5672/ \
    AMQP_QUEUE=tickets.triage

EXPOSE 8080
ENTRYPOINT ["/app/server"]
