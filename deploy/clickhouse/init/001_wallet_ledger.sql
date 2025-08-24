-- ===== 0) Database =====
CREATE DATABASE IF NOT EXISTS smsgw;

-- ===== 1) Clean old objects (safe order) =====
DROP VIEW  IF EXISTS smsgw.mv_wallet_ledger_daily;
DROP VIEW  IF EXISTS smsgw.mv_wallet_ledger_consumer;
DROP TABLE IF EXISTS smsgw.wallet_ledger_kafka;
DROP TABLE IF EXISTS smsgw.wallet_ledger;

-- ===== 2) Final table (ClickHouse, keep 30 days) =====
CREATE TABLE smsgw.wallet_ledger
(
    customer_id UInt64,
    op          Enum8('topup'=1,'reserve'=2,'capture'=3,'refund'=4),
    amount      UInt64,
    message_id  String,
    created_at  DateTime
)
    ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (customer_id, created_at)
TTL created_at + INTERVAL 30 DAY
SETTINGS ttl_only_drop_parts = 1;

-- ===== 3) Kafka source
CREATE TABLE smsgw.wallet_ledger_kafka
(
    customer_id     UInt64,
    op              String,
    amount          UInt64,
    message_id      Nullable(String),
    idempotency_key String,
    created_at      Nullable(UInt64),   -- epoch ms (e.g. 1756073554000)
    __op            Nullable(String),   -- 'c' | 'u' | 'd'
    __ts_ms         Nullable(UInt64)    -- epoch ms from Debezium
)
    ENGINE = Kafka
SETTINGS
  kafka_broker_list       = 'smsgw-kafka:9092',
  kafka_topic_list        = 'dbz.smsgw.wallet_ledger',
  kafka_group_name        = 'ch-ledger-consumer',
  kafka_format            = 'JSONEachRow',
  kafka_num_consumers     = 2,
  kafka_handle_error_mode = 'stream';

-- ===== 4) MV: ingest Kafka -> MergeTree (only inserts) =====
CREATE MATERIALIZED VIEW smsgw.mv_wallet_ledger_consumer
TO smsgw.wallet_ledger
AS
SELECT
    toUInt64(customer_id) AS customer_id,
    CAST(op, 'Enum8(\'topup\'=1,\'reserve\'=2,\'capture\'=3,\'refund\'=4)') AS op,
    toUInt64(amount) AS amount,
    ifNull(message_id, '') AS message_id,
    toDateTime(coalesce(created_at, __ts_ms, toUInt64(0)) / 1000) AS created_at
FROM smsgw.wallet_ledger_kafka
WHERE __op = 'c' OR __op IS NULL;

-- ===== 5) Optional: daily rollup for fast reports =====
CREATE MATERIALIZED VIEW smsgw.mv_wallet_ledger_daily
ENGINE = SummingMergeTree
PARTITION BY toYYYYMM(date)
ORDER BY (customer_id, date)
AS
SELECT
    customer_id,
    toDate(created_at) AS date,
  sumIf(amount, op='topup')   AS topup_sum,
  sumIf(amount, op='reserve') AS reserve_sum,
  sumIf(amount, op='capture') AS capture_sum,
  sumIf(amount, op='refund')  AS refund_sum
FROM smsgw.wallet_ledger
GROUP BY customer_id, date;