# SMS Gateway — Architecture Overview

## 1) Goals
- High-throughput SMS delivery (100M+/day).
- Customer **wallet** with `balance` / `reserved` and safe money flow: **topup → reserve → capture / refund**.
- Full **ledger** (append-only) for audit & reconciliation.
- Clear OLTP (**MySQL**) vs Analytics (**ClickHouse**) separation.
- At-least-once delivery with **Transactional Outbox → Kafka**.

---

## 2) High-Level Components
- **HTTP API (Echo)**  
  Exposes endpoints for sending SMS and topping up wallet.
- **Outbox + Kafka**  
  `messages` and `outbox` are written atomically. Outbox relayed to Kafka.
- **Workers (Sender)**  
  Kafka consumers per lane (`sms.normal`, `sms.express`). Dispatch to providers, batch DB updates.
- **Wallet & Ledger**  
  `wallet_accounts` = state, `wallet_ledger` = history.
- **Databases**
    - **MySQL** = transactional store.
    - **ClickHouse** = analytics via Debezium CDC.

---

## 3) API Endpoints

### POST /v1/sms/send
**Request**
```json
{ "phone": "09121234567", "text": "Hello world", "type": "normal" }
```

**Flow**
- Deduct from balance → move to reserved.
- Insert messages + wallet_ledger(reserve) + outbox.
- Publish to Kafka.

**Response**
```json
{ "enqueued": true, "id": "01K3...", "customer_id": "123", "type": "normal" }
```

---

### POST /v1/wallet/topup
**Request**
```json
{ "amount": 50000, "request_id": "uuid-or-transaction-id" }
```

**Flow**
- Insert wallet_ledger(topup) with idempotency key.
- Increase balance.
- If already processed → idempotent=true.

**Response**
```json
{ "topup": true, "idempotent": false, "amount": 50000, "customer_id": 123 }
```

---

### GET /v1/reports/messages
Fetch historical messages (ClickHouse). Supports filters.

---

## 4) Database Schema

### MySQL (OLTP)

**wallet_accounts**
```
customer_id BIGINT PK,
balance BIGINT,
reserved BIGINT,
created_at, updated_at
```

**wallet_ledger**
```
id BIGINT PK AUTO_INCREMENT,
customer_id BIGINT,
op ENUM('topup','reserve','capture','refund'),
amount BIGINT,
message_id VARCHAR(64),
idempotency_key VARCHAR(128) UNIQUE,
created_at DATETIME
```

**messages**
```
id VARCHAR(26) PK, -- ULID
customer_id BIGINT,
phone VARCHAR(32),
text TEXT,
type ENUM('normal','express'),
status ENUM('queued','sent','failed'),
created_at, updated_at
```

**outbox**
- Transactional outbox for Kafka events.

---

### ClickHouse (Analytics)

**wallet_ledger**
- Same as MySQL, TTL = 30 days.
- Ingested via Debezium CDC → Kafka → CH.

**messages_log**
- Append-only message status changes.

**mv_wallet_ledger_daily**
- Materialized view, daily sums per customer.

---

## 5) Processing Flow
1. Client → Send API → enqueue SMS → insert rows → outbox → Kafka.
2. Sender Worker → consume Kafka → send to providers → update messages + append ledger (capture/refund).
3. Wallet always updated atomically.
4. Debezium CDC streams MySQL → Kafka → ClickHouse.
5. Analytics served from ClickHouse.

---

## 6) Concurrency & Safety
- Wallet updates use row-level locks (FOR UPDATE) to avoid races.
- Workers do batch updates (~200 messages).
- Idempotency keys prevent duplicates.
- ClickHouse TTL (30 days) controls storage.

---

## 7) Curl Examples

**Topup**
```bash
curl -X POST http://localhost:8080/v1/wallet/topup \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: KEY' \
  -d '{"amount":50000,"request_id":"123e4567"}'
```

**Send SMS**
```bash
curl -X POST http://localhost:8080/v1/sms/send \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: KEY' \
  -d '{"phone":"09121234567","text":"Hello","type":"normal"}'
```

---

## 8) Makefile Quick Reference
```
make run-server          # start HTTP server
make run-sender-normal   # start normal lane worker
make run-sender-express  # start express lane worker
make migrate             # run MySQL migrations
make seed                # seed demo data
make up / make down      # docker-compose helpers
```

---

## 9) Security
- API key auth.
- Customers can only modify their own wallet.
- Every financial effect has a matching ledger row (audit).
- Internal errors hidden from clients; logs include trace IDs.


## Architecture Diagram

```mermaid
flowchart TD

  %% --- Client & API ---
  client([Client])
  api[API Layer]

  client -->|/v1/sms/send| api
  client -->|/v1/wallet/topup| api

  %% --- Database OLTP ---
  api -->|insert messages + outbox + ledger| mysql[(MySQL)]
  mysql -->|Outbox relay| kafkaSMS[(Kafka sms.normal / sms.express)]

  %% --- Workers ---
  subgraph Workers
    direction LR
    wNormal[Sender Normal]
    wExpress[Sender Express]
  end

  kafkaSMS --> wNormal
  kafkaSMS --> wExpress

  %% --- Dispatcher + Providers ---
  subgraph Dispatcher
    direction LR
    dispatch[Dispatcher]
    pA[Provider A]
    pB[Provider B]
    pC[Provider C]
    dispatch --> pA
    dispatch --> pB
    dispatch --> pC
  end

  wNormal --> dispatch
  wExpress --> dispatch

  wNormal -->|batch update status + wallet| mysql
  wExpress -->|batch update status + wallet| mysql

  %% --- Analytics ---
  mysql -->|CDC| kafkaCDC[(Kafka CDC)]
  kafkaCDC --> ch[(ClickHouse)]