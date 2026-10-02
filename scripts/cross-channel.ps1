$ErrorActionPreference = "Stop"

function Wait-Ready($Uri) {
    for ($i = 0; $i -lt 60; $i++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing "$Uri/health/ready"
            if ($response.StatusCode -eq 200) { return }
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "API not ready: $Uri"
}

function Token($ClientId, $Secret) {
    for ($i = 0; $i -lt 60; $i++) {
        try {
            $token = (Invoke-RestMethod -Method Post `
                -Uri "http://localhost:8085/realms/jungle/protocol/openid-connect/token" `
                -ContentType "application/x-www-form-urlencoded" `
                -Body "grant_type=client_credentials&client_id=$ClientId&client_secret=$Secret").access_token
            if ($token) { return $token }
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "Could not obtain token for $ClientId"
}

Write-Host "Starting stack..."
docker compose up -d --build
if ($LASTEXITCODE -ne 0) { throw "docker compose failed" }
Wait-Ready "http://localhost:8080"

$walletToken = Token "wallet-service" "wallet-service-secret"
$providerToken = Token "provider-a" "provider-a-secret"
$playerId = [guid]::NewGuid().ToString()

$wallet = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wallets" `
    -Headers @{ Authorization = "Bearer $walletToken" } `
    -ContentType "application/json" `
    -Body (@{ playerId = $playerId; initialBalance = @{ amount = "100.00"; currency = "BRL" } } | ConvertTo-Json -Depth 5)

$externalId = [guid]::NewGuid().ToString()
$idempotencyKey = "provider-a:$externalId"
$messageId = "msg-$([guid]::NewGuid().ToString())"
$occurredAt = [DateTime]::UtcNow.ToString("o")

$business = @{
    providerId = "provider-a"
    externalTransactionId = $externalId
    idempotencyKey = $idempotencyKey
    playerId = $playerId
    walletId = $wallet.id
    roundId = "round-cross-channel"
    gameId = "fortune-chimp"
    kind = "BET"
    money = @{ amount = "10.00"; currency = "BRL" }
}

$envelope = @{
    messageId = $messageId
    type = "WagerTransactionRequested"
    occurredAt = $occurredAt
    data = $business
} | ConvertTo-Json -Depth 10 -Compress

$envelopeBase64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($envelope))
docker compose exec -T localstack sh -c "printf '%s' '$envelopeBase64' | base64 -d > /tmp/cross-channel.json"
if ($LASTEXITCODE -ne 0) { throw "Could not stage SQS message" }

$walletId = $wallet.id
docker compose exec -T localstack sh -c 'QUEUE_URL=$(awslocal sqs get-queue-url --queue-name wager-transactions.fifo --query QueueUrl --output text); awslocal sqs send-message --queue-url "$QUEUE_URL" --message-body file:///tmp/cross-channel.json --message-group-id "$1" --message-deduplication-id "$2" >/dev/null' sh $walletId $messageId
if ($LASTEXITCODE -ne 0) { throw "Could not send SQS message" }

$transaction = $null
$terminalFailure = $null
for ($i = 0; $i -lt 120; $i++) {
    try {
        $transaction = Invoke-RestMethod -Method Get `
            -Uri "http://localhost:8080/providers/provider-a/wagering/transactions/$externalId" `
            -Headers @{ Authorization = "Bearer $providerToken" }
        if ($transaction.status -eq "PROCESSED") { break }
        if ($transaction.status -eq "REJECTED") {
            $terminalFailure = "SQS transaction was rejected: $($transaction.reasonCode)"
            break
        }
    } catch {
        # 404 is expected while the consumer has not committed the message yet.
    }
    Start-Sleep -Seconds 1
}
if ($terminalFailure) { throw $terminalFailure }
if ($null -eq $transaction -or $transaction.status -ne "PROCESSED") {
    $inbox = docker compose exec -T postgres psql -U jungle -d jungle_gaming -At -F '|' -c "SELECT status, attempts, COALESCE(last_error,'') FROM inbox_messages WHERE consumer_name='wager-transactions' AND message_id='$messageId';"
    $queue = docker compose exec -T localstack sh -c 'QUEUE_URL=$(awslocal sqs get-queue-url --queue-name wager-transactions.fifo --query QueueUrl --output text); awslocal sqs get-queue-attributes --queue-url "$QUEUE_URL" --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible --output json'
    Write-Host "Cross-channel diagnostic - inbox: $((($inbox | Out-String).Trim()))"
    Write-Host "Cross-channel diagnostic - queue: $((($queue | Out-String).Trim()))"
    docker compose logs --tail=80 api
    throw "SQS transaction was not processed within 120 seconds"
}

$httpBody = @{
    providerId = "provider-a"
    externalTransactionId = $externalId
    playerId = $playerId
    walletId = $wallet.id
    roundId = "round-cross-channel"
    gameId = "fortune-chimp"
    kind = "BET"
    money = @{ amount = "10.00"; currency = "BRL" }
} | ConvertTo-Json -Depth 5

$replay = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wagering/transactions" `
    -Headers @{ Authorization = "Bearer $providerToken"; "Idempotency-Key" = $idempotencyKey } `
    -ContentType "application/json" -Body $httpBody

if (-not $replay.idempotentReplay) { throw "HTTP did not replay the SQS operation" }
if ($replay.balance.amount -ne "90.00") { throw "Expected replay balance 90.00, got $($replay.balance.amount)" }

$finalWallet = Invoke-RestMethod -Method Get -Uri "http://localhost:8080/wallets/$($wallet.id)" `
    -Headers @{ Authorization = "Bearer $walletToken" }
if ($finalWallet.balance.amount -ne "90.00") { throw "Expected final balance 90.00, got $($finalWallet.balance.amount)" }

$ledger = Invoke-RestMethod -Method Get -Uri "http://localhost:8080/wallets/$($wallet.id)/ledger?limit=50" `
    -Headers @{ Authorization = "Bearer $walletToken" }
$debits = @($ledger.items | Where-Object { $_.direction -eq "DEBIT" })
if ($debits.Count -ne 1) { throw "Expected exactly one debit, got $($debits.Count)" }

Write-Host "CROSS-CHANNEL PASS: SQS processed once, HTTP replayed it, final balance 90.00 and one debit."
