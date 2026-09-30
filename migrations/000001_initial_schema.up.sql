CREATE TABLE wallets (
    id TEXT PRIMARY KEY,
    player_id TEXT NOT NULL,
    currency CHAR(3) NOT NULL,
    balance BIGINT NOT NULL,
    version BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT wallets_player_currency_unique
        UNIQUE (player_id, currency),

    CONSTRAINT wallets_balance_non_negative
        CHECK (balance >= 0),

    CONSTRAINT wallets_version_positive
        CHECK (version >= 1),

    CONSTRAINT wallets_currency_format
        CHECK (currency ~ '^[A-Z]{3}$'),

    CONSTRAINT wallets_timestamps_valid
        CHECK (updated_at >= created_at)
);

CREATE TABLE wager_transactions (
    id TEXT PRIMARY KEY,
    external_transaction_id TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    payload_hash TEXT NOT NULL,

    wallet_id TEXT NOT NULL,
    player_id TEXT NOT NULL,
    round_id TEXT NOT NULL,
    game_id TEXT NOT NULL,

    kind TEXT NOT NULL,
    status TEXT NOT NULL,

    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,

    reference_external_transaction_id TEXT,
    reference_transaction_id TEXT,

    failure_code TEXT,
    result_balance BIGINT,

    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT wager_transactions_wallet_fk
        FOREIGN KEY (wallet_id)
        REFERENCES wallets (id),

    CONSTRAINT wager_transactions_reference_transaction_fk
        FOREIGN KEY (reference_transaction_id)
        REFERENCES wager_transactions (id),

    CONSTRAINT wager_transactions_provider_external_unique
        UNIQUE (provider_id, external_transaction_id),

    CONSTRAINT wager_transactions_provider_idempotency_unique
        UNIQUE (provider_id, idempotency_key),

    CONSTRAINT wager_transactions_kind_valid
        CHECK (
            kind IN (
                'OPENING',
                'BET',
                'WIN',
                'LOSS',
                'REFUND',
                'ROLLBACK'
            )
        ),

    CONSTRAINT wager_transactions_status_valid
        CHECK (
            status IN (
                'PENDING',
                'PENDING_REFERENCE',
                'PROCESSED',
                'REJECTED',
                'FAILED'
            )
        ),

    CONSTRAINT wager_transactions_amount_non_negative
        CHECK (amount >= 0),

    CONSTRAINT wager_transactions_currency_format
        CHECK (currency ~ '^[A-Z]{3}$'),

    CONSTRAINT wager_transactions_result_balance_non_negative
        CHECK (
            result_balance IS NULL
            OR result_balance >= 0
        ),

    CONSTRAINT wager_transactions_timestamps_valid
        CHECK (updated_at >= created_at),

    CONSTRAINT wager_transactions_reference_required
        CHECK (
            kind NOT IN ('REFUND', 'ROLLBACK')
            OR reference_external_transaction_id IS NOT NULL
        ),

    CONSTRAINT wager_transactions_failure_code_valid
        CHECK (
            (
                status IN ('REJECTED', 'FAILED')
                AND failure_code IS NOT NULL
            )
            OR
            (
                status NOT IN ('REJECTED', 'FAILED')
                AND failure_code IS NULL
            )
        )
);

CREATE INDEX wager_transactions_reference_external_idx
    ON wager_transactions (
        provider_id,
        reference_external_transaction_id
    )
    WHERE reference_external_transaction_id IS NOT NULL;

CREATE INDEX wager_transactions_reference_transaction_idx
    ON wager_transactions (reference_transaction_id)
    WHERE reference_transaction_id IS NOT NULL;

CREATE INDEX wager_transactions_pending_reference_idx
    ON wager_transactions (created_at, id)
    WHERE status = 'PENDING_REFERENCE';

CREATE INDEX wager_transactions_wallet_created_idx
    ON wager_transactions (wallet_id, created_at, id);

CREATE TABLE ledger_entries (
    id TEXT PRIMARY KEY,
    wallet_id TEXT NOT NULL,
    transaction_id TEXT NOT NULL,

    direction TEXT NOT NULL,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,

    balance_before BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,

    created_at TIMESTAMPTZ NOT NULL,

    CONSTRAINT ledger_entries_wallet_fk
        FOREIGN KEY (wallet_id)
        REFERENCES wallets (id),

    CONSTRAINT ledger_entries_transaction_fk
        FOREIGN KEY (transaction_id)
        REFERENCES wager_transactions (id),

    CONSTRAINT ledger_entries_wallet_transaction_unique
        UNIQUE (wallet_id, transaction_id),

    CONSTRAINT ledger_entries_direction_valid
        CHECK (direction IN ('CREDIT', 'DEBIT')),

    CONSTRAINT ledger_entries_amount_positive
        CHECK (amount > 0),

    CONSTRAINT ledger_entries_currency_format
        CHECK (currency ~ '^[A-Z]{3}$'),

    CONSTRAINT ledger_entries_balance_before_non_negative
        CHECK (balance_before >= 0),

    CONSTRAINT ledger_entries_balance_after_non_negative
        CHECK (balance_after >= 0),

    CONSTRAINT ledger_entries_balance_equation
        CHECK (
            (
                direction = 'CREDIT'
                AND balance_after = balance_before + amount
            )
            OR
            (
                direction = 'DEBIT'
                AND balance_after = balance_before - amount
            )
        )
);

CREATE INDEX ledger_entries_wallet_created_idx
    ON ledger_entries (wallet_id, created_at, id);

CREATE OR REPLACE FUNCTION prevent_ledger_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'ledger entries are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER ledger_entries_prevent_update
BEFORE UPDATE ON ledger_entries
FOR EACH ROW
EXECUTE FUNCTION prevent_ledger_mutation();

CREATE TRIGGER ledger_entries_prevent_delete
BEFORE DELETE ON ledger_entries
FOR EACH ROW
EXECUTE FUNCTION prevent_ledger_mutation();