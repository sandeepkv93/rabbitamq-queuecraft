# rabbitamq-queuecraft

A production-oriented Go sample that combines:
- `net/http` REST API
- RabbitMQ queueing via `github.com/rabbitmq/amqp091-go`
- asynchronous ticket triage worker
- containerized runtime with Docker Compose

## Why this example is interesting

The API accepts support tickets and returns `202 Accepted` immediately. A background worker consumes each ticket from RabbitMQ, classifies priority (`high|medium|low`) based on incident signals, and updates ticket handling guidance asynchronously.

This demonstrates queue-backed decoupling between request handling and business processing.

## Project structure

- `cmd/server`: process entrypoint
- `internal/config`: environment configuration
- `internal/app`: app composition and lifecycle
- `internal/httpapi`: REST handlers using `net/http`
- `internal/mq`: RabbitMQ adapter
- `internal/service`: ticket domain service and triage logic
- `internal/store`: in-memory ticket store
- `internal/worker`: queue consumer runner

## Run locally (Go)

```bash
docker compose up -d rabbitmq
AMQP_URL=amqp://guest:guest@localhost:5672/ go run ./cmd/server
```

## Run full stack (Docker Compose)

```bash
docker compose up --build
```

Services:
- API: `http://localhost:18080`
- RabbitMQ management UI: `http://localhost:15672` (`guest` / `guest`)

## API usage

Create ticket:

```bash
curl -sS -X POST http://localhost:18080/v1/tickets \
  -H 'Content-Type: application/json' \
  -d '{
    "customer_id": "cust_123",
    "subject": "Payment failed during checkout",
    "body": "Urgent: multiple card attempts failed with timeout"
  }'
```

Fetch ticket status:

```bash
curl -sS http://localhost:18080/v1/tickets/<ticket_id>
```

Health check:

```bash
curl -sS http://localhost:18080/healthz
```

## Environment variables

- `HTTP_ADDR` (default `:8080`)
- `APP_MODE` (`all|api|worker`, default `all`)
- `AMQP_URL` (default `amqp://guest:guest@rabbitmq:5672/`)
- `AMQP_QUEUE` (default `tickets.triage`)
- `WORKER_SLEEP_MS` (default `1200`)
- `AMQP_MAX_RETRIES` (default `20`)
- `AMQP_RETRY_BACKOFF_SECONDS` (default `2`)
- `SHUTDOWN_TIMEOUT_SECONDS` (default `15`)

## Quality checks

```bash
go test ./...
go build ./cmd/server
```
