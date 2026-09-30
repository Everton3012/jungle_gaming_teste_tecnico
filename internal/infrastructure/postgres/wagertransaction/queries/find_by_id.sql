SELECT
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
FROM wager_transactions
WHERE id = $1;