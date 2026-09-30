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
WHERE id = $1;