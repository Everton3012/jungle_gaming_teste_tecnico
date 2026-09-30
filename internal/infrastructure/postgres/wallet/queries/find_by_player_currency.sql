SELECT
    id,
    player_id,
    currency,
    balance,
    version,
    created_at,
    updated_at
FROM wallets
WHERE player_id = $1
  AND currency = $2;