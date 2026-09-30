SELECT
    id,
    wallet_id,
    transaction_id,
    direction,
    amount,
    currency,
    balance_before,
    balance_after,
    created_at
FROM ledger_entries
WHERE transaction_id = $1
ORDER BY created_at ASC, id ASC;