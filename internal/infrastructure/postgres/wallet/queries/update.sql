UPDATE wallets
SET
    balance = $2,
    version = $3,
    updated_at = $4
WHERE id = $1
  AND version = $5;