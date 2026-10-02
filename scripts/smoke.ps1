$ErrorActionPreference = "Stop"

function Wait-Until($Description, [scriptblock]$Action, [int]$Attempts = 60) {
    for ($i = 0; $i -lt $Attempts; $i++) {
        try {
            $result = & $Action
            if ($null -ne $result) { return $result }
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "Timeout waiting for $Description"
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
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }

Wait-Until "API readiness" {
    $response = Invoke-WebRequest -UseBasicParsing "http://localhost:8080/health/ready"
    if ($response.StatusCode -eq 200) { return $response }
    return $null
} | Out-Null

$walletToken = Wait-Until "Keycloak wallet token" { Token "wallet-service" "wallet-service-secret" }
$providerAToken = Token "provider-a" "provider-a-secret"
$providerBToken = Token "provider-b" "provider-b-secret"

# Authentication is effective on business endpoints.
try {
    Invoke-RestMethod -Method Get -Uri "http://localhost:8080/wallets/not-authorized" | Out-Null
    throw "Unauthenticated wallet request unexpectedly succeeded"
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 401) { throw }
}
try {
    Invoke-RestMethod -Method Get -Uri "http://localhost:8080/wallets/not-authorized" `
        -Headers @{ Authorization = "Bearer $providerAToken" } | Out-Null
    throw "Provider accessed an internal wallet endpoint"
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 403) { throw }
}

$playerId = [guid]::NewGuid().ToString()
$walletBody = @{
    playerId = $playerId
    initialBalance = @{ amount = "100.00"; currency = "BRL" }
} | ConvertTo-Json -Depth 5

$wallet = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wallets" `
    -Headers @{ Authorization = "Bearer $walletToken" } `
    -ContentType "application/json" -Body $walletBody

if ($wallet.balance.amount -ne "100.00" -or $wallet.version -ne 1) {
    throw "Unexpected wallet creation result"
}

$externalId = [guid]::NewGuid().ToString()
$betBody = @{
    providerId = "provider-a"
    externalTransactionId = $externalId
    playerId = $playerId
    walletId = $wallet.id
    roundId = "round-smoke"
    gameId = "fortune-chimp"
    kind = "BET"
    money = @{ amount = "25.00"; currency = "BRL" }
} | ConvertTo-Json -Depth 5

$headers = @{
    Authorization = "Bearer $providerAToken"
    "Idempotency-Key" = "provider-a:$externalId"
}

$first = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wagering/transactions" `
    -Headers $headers -ContentType "application/json" -Body $betBody
if ($first.status -ne "PROCESSED" -or $first.balance.amount -ne "75.00" -or $first.idempotentReplay) {
    throw "Unexpected first BET result"
}

$replay = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wagering/transactions" `
    -Headers $headers -ContentType "application/json" -Body $betBody
if (-not $replay.idempotentReplay -or $replay.balance.amount -ne "75.00") {
    throw "Idempotent replay failed"
}

$own = Invoke-RestMethod -Method Get `
    -Uri "http://localhost:8080/providers/provider-a/wagering/transactions/$externalId" `
    -Headers @{ Authorization = "Bearer $providerAToken" }
if ($own.providerId -ne "provider-a") { throw "Provider A lookup failed" }

try {
    Invoke-RestMethod -Method Get `
        -Uri "http://localhost:8080/providers/provider-a/wagering/transactions/$externalId" `
        -Headers @{ Authorization = "Bearer $providerBToken" } | Out-Null
    throw "Provider isolation failed: provider-b accessed provider-a"
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 403) { throw }
}

$reconciliation = Invoke-RestMethod -Method Post `
    -Uri "http://localhost:8080/wallets/$($wallet.id)/reconciliation" `
    -Headers @{ Authorization = "Bearer $walletToken" }
if (-not $reconciliation.consistent -or $reconciliation.storedBalance.amount -ne "75.00") {
    throw "Reconciliation failed"
}

$metrics = Invoke-WebRequest -UseBasicParsing "http://localhost:8080/metrics"
if ($metrics.StatusCode -ne 200 -or $metrics.Content -notmatch "jungle_wager_processed_total") {
    throw "Metrics endpoint failed"
}

Write-Host "SMOKE PASS: OAuth2, provider isolation, wallet, BET, replay, reconciliation and metrics."
