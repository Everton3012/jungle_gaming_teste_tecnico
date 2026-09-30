INSERT INTO wager_transactions (
    id,
    external_transaction_id,
    provider_id,
    idempotency_key,
    payload_hash,
    wallet_id,
    player_id,
    round_id,
    game_id,
    kind,
    status,
    amount,
    currency,
    reference_external_transaction_id,
    reference_transaction_id,
    failure_code,
    result_balance,
    created_at,
    updated_at
)
VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15,
    $16, $17, $18, $19
);