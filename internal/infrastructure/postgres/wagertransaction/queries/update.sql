UPDATE wager_transactions
SET
    status = $2,
    reference_transaction_id = $3,
    failure_code = $4,
    result_balance = $5,
    updated_at = $6
WHERE id = $1;