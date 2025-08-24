SET
FOREIGN_KEY_CHECKS = 0;
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS wallet_accounts;
DROP TABLE IF EXISTS wallet_ledger;
DROP TABLE IF EXISTS customers;
SET
FOREIGN_KEY_CHECKS = 1;

-- customers
CREATE TABLE customers
(
    id             BIGINT AUTO_INCREMENT PRIMARY KEY,
    name           VARCHAR(120) NOT NULL,
    api_key        CHAR(32)     NOT NULL UNIQUE,
    status         ENUM('active','suspended') NOT NULL DEFAULT 'active',
    rate_limit_rps INT NULL,
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- wallet_accounts
CREATE TABLE wallet_accounts
(
    customer_id BIGINT   NOT NULL PRIMARY KEY,
    balance     BIGINT   NOT NULL DEFAULT 0,
    reserved    BIGINT   NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_wallet_customer
        FOREIGN KEY (customer_id) REFERENCES customers (id)
            ON UPDATE RESTRICT ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- messages
CREATE TABLE messages
(
    id          CHAR(26)    NOT NULL PRIMARY KEY, -- ULID
    customer_id BIGINT      NOT NULL,
    phone       VARCHAR(32) NOT NULL,
    text        TEXT        NOT NULL,
    type        ENUM('normal','express') NOT NULL DEFAULT 'normal',
    status      ENUM('queued','sent','failed') NOT NULL DEFAULT 'queued',
    created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_messages_customer
        FOREIGN KEY (customer_id) REFERENCES customers (id)
            ON UPDATE RESTRICT ON DELETE RESTRICT,
    KEY         idx_customer_created (customer_id, created_at),
    KEY         idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Minimal outbox for Debezium Outbox SMT
CREATE TABLE outbox
(
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    aggregate    VARCHAR(32)  NOT NULL, -- e.g. "message"
    aggregate_id CHAR(26)     NOT NULL, -- ULID
    topic        VARCHAR(120) NOT NULL, -- sms.normal | sms.express
    payload      JSON         NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY          idx_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- wallet_ledger
CREATE TABLE wallet_ledger
(
    id              BIGINT       NOT NULL AUTO_INCREMENT,
    customer_id     BIGINT       NOT NULL,
    op              ENUM('topup','reserve','capture','refund') NOT NULL,
    amount          BIGINT       NOT NULL,
    message_id      VARCHAR(64) NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_idem (idempotency_key),
    KEY             idx_cust_time (customer_id, created_at),
    KEY             idx_msg (message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
