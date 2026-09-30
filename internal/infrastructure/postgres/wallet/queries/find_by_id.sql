SELECT
    id,
    player_id,
    currency,
    balance,
    version,
    created_at,
    updated_at
FROM wallets
WHERE id = $1;