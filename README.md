# RabbitAMQ QueueCraft

A Go project that combines:
- `go-chi` REST API router
- RabbitMQ queueing via `github.com/rabbitmq/amqp091-go` with retry + dead-letter queues
- PostgreSQL as the ticket source of truth
- asynchronous ticket triage worker
- containerized runtime with Docker Compose

## Flow Diagram

```mermaid
flowchart LR
    C[Client] -->|POST /v1/tickets| API[Go chi API]
    API -->|INSERT ticket status=queued| DB[(PostgreSQL)]
    API -->|Publish ticket message| Q[(Main queue: tickets.triage)]
    W[Worker] -->|Consume message| Q
    W -->|Classify priority + action| L[Ticket Logic]
    W -->|On failure, republish| R[(Retry queue: tickets.triage.retry)]
    R -->|TTL backoff expires| Q
    W -->|Exceeds max retries| D[(Dead-letter queue: tickets.triage.dead)]
    L -->|UPDATE ticket status=completed| DB
    C -->|GET /v1/tickets/:id| API
    API -->|SELECT latest ticket status| DB
    API -->|Return latest ticket state| C
```

## Why this example is interesting

The API accepts support tickets and returns `202 Accepted` immediately. A background worker consumes each ticket from RabbitMQ, classifies priority (`high|medium|low`) based on incident signals, and updates ticket handling guidance asynchronously.

This demonstrates queue-backed decoupling between request handling and business processing with persistent ticket state in PostgreSQL.

RabbitMQ policy in this project:
- Main queue: `tickets.triage`
- Retry queue: `tickets.triage.retry` (message TTL backoff)
- Dead-letter queue: `tickets.triage.dead` (for exhausted retries)

## Project structure

- `cmd/server`: process entrypoint
- `internal/config`: environment configuration
- `internal/app`: app composition and lifecycle
- `internal/httpapi`: REST handlers and `chi` routes
- `internal/mq`: RabbitMQ adapter
- `internal/service`: ticket domain service and triage logic
- `internal/store`: storage adapters (`postgres` + in-memory test store)
- `internal/worker`: queue consumer runner

## Run locally (Go)

```bash
docker compose up -d postgres rabbitmq
cp .env.example .env
go run ./cmd/server
```

The app loads environment variables from `.env` via `github.com/joho/godotenv`.

## Run full stack (Docker Compose)

```bash
docker compose up --build
```

Services:
- API: `http://localhost:18080`
- PostgreSQL: `localhost:5433` (`postgres` / `postgres`, db: `tickets`)
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

## Generate many tickets (queue load script)

Use the included shell script to generate a burst of tickets and observe queue behavior:

```bash
./scripts/load-tickets.sh -n 200 -c 25
```

Options:
- `-u` API base URL (default `http://localhost:18080`)
- `-n` total tickets (default `50`)
- `-c` parallel requests/workers (default `10`)
- `-p` customer id prefix

## Environment variables

- `HTTP_ADDR` (default `:8080`)
- `APP_MODE` (`all|api|worker`, default `all`)
- `DATABASE_URL` (default `postgres://postgres:postgres@postgres:5432/tickets?sslmode=disable`)
- `DB_MAX_RETRIES` (default `20`)
- `DB_RETRY_BACKOFF_SECONDS` (default `2`)
- `AMQP_URL` (default `amqp://guest:guest@rabbitmq:5672/`)
- `AMQP_QUEUE` (default `tickets.triage`)
- `AMQP_MESSAGE_MAX_RETRIES` (default `3`)
- `AMQP_MESSAGE_RETRY_DELAY_MS` (default `2000`)
- `WORKER_SLEEP_MS` (default `1200`)
- `AMQP_MAX_RETRIES` (default `20`)
- `AMQP_RETRY_BACKOFF_SECONDS` (default `2`)
- `SHUTDOWN_TIMEOUT_SECONDS` (default `15`)

## Quality checks

```bash
go test ./...
go build ./cmd/server
```
