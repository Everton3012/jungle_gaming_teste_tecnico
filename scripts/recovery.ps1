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

function Post-Wager($TokenValue, $IdempotencyKey, $Body) {
    return Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wagering/transactions" `
        -Headers @{ Authorization = "Bearer $TokenValue"; "Idempotency-Key" = $IdempotencyKey } `
        -ContentType "application/json" -Body $Body
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

# 1) Persist a reversal before its reference.
$betExternalId = [guid]::NewGuid().ToString()
$refundExternalId = [guid]::NewGuid().ToString()
$refundBody = @{
    providerId = "provider-a"
    externalTransactionId = $refundExternalId
    playerId = $playerId
    walletId = $wallet.id
    roundId = "round-recovery"
    gameId = "fortune-chimp"
    kind = "REFUND"
    money = @{ amount = "20.00"; currency = "BRL" }
    referenceExternalTransactionId = $betExternalId
} | ConvertTo-Json -Depth 5

$pending = Post-Wager $providerToken "provider-a:$refundExternalId" $refundBody
if ($pending.status -ne "PENDING_REFERENCE") { throw "Expected PENDING_REFERENCE, got $($pending.status)" }
$pendingTransactionId = $pending.transactionId

# 2) Restart the application after the pending state is durable.
docker compose stop api
if ($LASTEXITCODE -ne 0) { throw "Could not stop API" }
docker compose start api
if ($LASTEXITCODE -ne 0) { throw "Could not restart API" }
Wait-Ready "http://localhost:8080"

# 3) Create the missing reference after restart.
$betBody = @{
    providerId = "provider-a"
    externalTransactionId = $betExternalId
    playerId = $playerId
    walletId = $wallet.id
    roundId = "round-recovery"
    gameId = "fortune-chimp"
    kind = "BET"
    money = @{ amount = "20.00"; currency = "BRL" }
} | ConvertTo-Json -Depth 5
$bet = Post-Wager $providerToken "provider-a:$betExternalId" $betBody
if ($bet.status -ne "PROCESSED") { throw "Reference BET did not process" }

# 4) The restarted reference worker must finish the previously pending REFUND.
$resolved = $null
for ($i = 0; $i -lt 60; $i++) {
    try {
        $resolved = Invoke-RestMethod -Method Get `
            -Uri "http://localhost:8080/wagering/transactions/$pendingTransactionId" `
            -Headers @{ Authorization = "Bearer $providerToken" }
        if ($resolved.status -eq "PROCESSED") { break }
    } catch {}
    Start-Sleep -Seconds 1
}
if ($null -eq $resolved -or $resolved.status -ne "PROCESSED") {
    throw "Pending REFUND was not recovered after restart"
}

$finalWallet = Invoke-RestMethod -Method Get -Uri "http://localhost:8080/wallets/$($wallet.id)" `
    -Headers @{ Authorization = "Bearer $walletToken" }
if ($finalWallet.balance.amount -ne "100.00") {
    throw "Expected balance restored to 100.00, got $($finalWallet.balance.amount)"
}

# 5) Force a synthetic abandoned outbox lease and prove another publisher reclaims it.
$eventId = "recovery-$([guid]::NewGuid().ToString())"
$payload = '{"eventId":"' + $eventId + '","eventType":"WagerTransactionProcessed","aggregateId":"recovery-aggregate","correlationId":"recovery","occurredAt":"' + ([DateTime]::UtcNow.ToString("o")) + '","version":1,"data":{"transactionId":"recovery-aggregate"}}'
$escapedPayload = $payload.Replace("'", "''")
$sql = "INSERT INTO outbox_events (id,event_type,aggregate_type,aggregate_id,payload,status,attempts,available_at,published_at,last_error,created_at,updated_at,locked_until) VALUES ('$eventId','WagerTransactionProcessed','WagerTransaction','recovery-aggregate','$escapedPayload'::jsonb,'PROCESSING',1,NOW()-INTERVAL '2 minutes',NULL,NULL,NOW()-INTERVAL '2 minutes',NOW()-INTERVAL '2 minutes',NOW()-INTERVAL '1 minute');"
$sql | docker compose exec -T postgres psql -U jungle -d jungle_gaming -v ON_ERROR_STOP=1 | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Could not create abandoned outbox event" }

$published = $false
for ($i = 0; $i -lt 60; $i++) {
    $status = docker compose exec -T postgres psql -U jungle -d jungle_gaming -At -c "SELECT status FROM outbox_events WHERE id='$eventId';"
    if (($status | Out-String).Trim() -eq "PUBLISHED") { $published = $true; break }
    Start-Sleep -Seconds 1
}
if (-not $published) { throw "Expired PROCESSING outbox event was not reclaimed" }

Write-Host "RECOVERY PASS: pending reference survived restart, resolved later, and abandoned outbox work was reclaimed."
