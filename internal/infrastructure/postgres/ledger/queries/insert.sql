INSERT INTO ledger_entries (
    id,
    wallet_id,
    transaction_id,
    direction,
    amount,
    currency,
    balance_before,
    balance_after,
    created_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9
);