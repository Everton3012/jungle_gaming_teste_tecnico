INSERT INTO wallets (
    id,
    player_id,
    currency,
    balance,
    version,
    created_at,
    updated_at
)
VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
);