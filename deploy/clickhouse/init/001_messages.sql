-- ===============================
-- (Debezium -> Kafka -> CH Aggregating)
-- ===============================

CREATE DATABASE IF NOT EXISTS smsgw;

DROP TABLE IF EXISTS smsgw.messages_latest_agg;
CREATE TABLE smsgw.messages_latest_agg
(
    id                 String,
    customer_id_state  AggregateFunction(argMax, UInt64,   DateTime64(3, 'UTC')),
    phone_state        AggregateFunction(argMax, String,   DateTime64(3, 'UTC')),
    text_state         AggregateFunction(argMax, String,   DateTime64(3, 'UTC')),
    type_state         AggregateFunction(argMax, String,   DateTime64(3, 'UTC')),
    status_state       AggregateFunction(argMax, String,   DateTime64(3, 'UTC')),
    created_at_state   AggregateFunction(argMax, DateTime, DateTime64(3, 'UTC')),
    updated_at_state   AggregateFunction(argMax, DateTime, DateTime64(3, 'UTC'))
)
    ENGINE = AggregatingMergeTree
ORDER BY id;

DROP TABLE IF EXISTS smsgw.kafka_messages;
CREATE TABLE smsgw.kafka_messages
(
    id           String,
    customer_id  UInt64,
    phone        String,
    text         String,
    type         String,
    status       String,
    created_at   UInt64,            -- unix ms
    updated_at   UInt64,            -- unix ms
    __op         Nullable(String),
    __ts_ms      Nullable(UInt64)
)
    ENGINE = Kafka
SETTINGS
  kafka_broker_list          = 'smsgw-kafka:9092',
  kafka_topic_list           = 'dbz.smsgw.messages',
  kafka_group_name           = 'ch-smsgw-messages-agg',
  kafka_format               = 'JSONEachRow',
  kafka_num_consumers        = 1,
  kafka_thread_per_consumer  = 1,
  kafka_handle_error_mode    = 'stream',
  kafka_max_block_size       = 10000;

DROP VIEW IF EXISTS smsgw.mv_messages_latest;
CREATE MATERIALIZED VIEW smsgw.mv_messages_latest
TO smsgw.messages_latest_agg
AS
SELECT
    id,
    argMaxState(customer_id, toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS customer_id_state,
    argMaxState(phone,       toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS phone_state,
    argMaxState(text,        toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS text_state,
    argMaxState(type,        toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS type_state,
    argMaxState(status,      toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS status_state,
    argMaxState(toDateTime(created_at/1000, 'UTC'),  toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS created_at_state,
    argMaxState(toDateTime(updated_at/1000, 'UTC'),  toDateTime64(coalesce(__ts_ms, updated_at)/1000.0, 3, 'UTC')) AS updated_at_state
FROM smsgw.kafka_messages
GROUP BY id;

CREATE OR REPLACE VIEW smsgw.messages_latest AS
SELECT
    id,
    argMaxMerge(customer_id_state) AS customer_id,
    argMaxMerge(phone_state)       AS phone,
    argMaxMerge(text_state)        AS text,
    argMaxMerge(type_state)        AS type,
    argMaxMerge(status_state)      AS status,
    argMaxMerge(created_at_state)  AS created_at,   -- UTC
    argMaxMerge(updated_at_state)  AS updated_at    -- UTC
FROM smsgw.messages_latest_agg
GROUP BY id;

CREATE OR REPLACE VIEW smsgw.messages_latest_irt AS
SELECT
    id,
    customer_id,
    phone,
    text,
    type,
    status,
    toTimeZone(created_at, 'Asia/Tehran') AS created_at_irt,
    toTimeZone(updated_at, 'Asia/Tehran') AS updated_at_irt
FROM smsgw.messages_latest;


