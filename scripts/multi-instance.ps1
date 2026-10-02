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

Write-Host "Starting three independent API processes..."
docker compose --profile multi up -d --build
if ($LASTEXITCODE -ne 0) { throw "docker compose failed" }

Wait-Ready "http://localhost:8080"
Wait-Ready "http://localhost:8081"
Wait-Ready "http://localhost:8082"

$walletToken = Token "wallet-service" "wallet-service-secret"
$providerToken = Token "provider-a" "provider-a-secret"
$playerId = [guid]::NewGuid().ToString()

$wallet = Invoke-RestMethod -Method Post -Uri "http://localhost:8080/wallets" `
    -Headers @{ Authorization = "Bearer $walletToken" } `
    -ContentType "application/json" `
    -Body (@{ playerId = $playerId; initialBalance = @{ amount = "100.00"; currency = "BRL" } } | ConvertTo-Json -Depth 5)

function BetPayload($ExternalId) {
    return (@{
        providerId = "provider-a"
        externalTransactionId = $ExternalId
        playerId = $playerId
        walletId = $wallet.id
        roundId = "round-concurrency"
        gameId = "fortune-chimp"
        kind = "BET"
        money = @{ amount = "80.00"; currency = "BRL" }
    } | ConvertTo-Json -Depth 5)
}

$id1 = [guid]::NewGuid().ToString()
$id2 = [guid]::NewGuid().ToString()
$body1 = BetPayload $id1
$body2 = BetPayload $id2

# Use HttpClient inside the background jobs so non-2xx responses are returned
# normally. Invoke-WebRequest throws on HTTP 422 and, depending on the
# PowerShell version, its exception response stream may already be consumed,
# which makes the response body appear empty and causes a false-negative here.
$wagerJob = {
    param($token, $id, $body, $uri)

    # Start-Job launches a separate Windows PowerShell process. On Windows
    # PowerShell 5.1 System.Net.Http is not guaranteed to be loaded in that
    # child process, so load it explicitly before resolving HttpClient types.
    Add-Type -AssemblyName System.Net.Http

    $client = New-Object System.Net.Http.HttpClient
    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Post, $uri)
    try {
        $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new("Bearer", $token)
        $request.Headers.Add("Idempotency-Key", "provider-a:$id")
        $request.Content = [System.Net.Http.StringContent]::new($body, [System.Text.Encoding]::UTF8, "application/json")

        $response = $client.SendAsync($request).GetAwaiter().GetResult()
        $responseBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        [pscustomobject]@{
            StatusCode = [int]$response.StatusCode
            Body = $responseBody
        }
    } finally {
        $request.Dispose()
        $client.Dispose()
    }
}

$job1 = Start-Job -ArgumentList $providerToken, $id1, $body1, "http://localhost:8080/wagering/transactions" -ScriptBlock $wagerJob
$job2 = Start-Job -ArgumentList $providerToken, $id2, $body2, "http://localhost:8081/wagering/transactions" -ScriptBlock $wagerJob

Wait-Job $job1, $job2 | Out-Null
$results = @((Receive-Job $job1), (Receive-Job $job2))
Remove-Job $job1, $job2

$processed = 0
$rejected = 0
foreach ($item in $results) {
    if ([string]::IsNullOrWhiteSpace($item.Body)) {
        throw "Concurrency request returned HTTP $($item.StatusCode) with an empty body. Results: $($results | ConvertTo-Json -Depth 5)"
    }

    try {
        $responseBody = $item.Body | ConvertFrom-Json
    } catch {
        throw "Concurrency request returned non-JSON body for HTTP $($item.StatusCode): $($item.Body)"
    }

    if ($item.StatusCode -eq 200 -and $responseBody.status -eq "PROCESSED") { $processed++ }
    if ($item.StatusCode -eq 422 -and $responseBody.status -eq "REJECTED" -and $responseBody.failureCode -eq "INSUFFICIENT_FUNDS") { $rejected++ }
}
if ($processed -ne 1 -or $rejected -ne 1) {
    throw "Expected one PROCESSED and one INSUFFICIENT_FUNDS rejection. Results: $($results | ConvertTo-Json -Depth 5)"
}

$finalWallet = Invoke-RestMethod -Method Get -Uri "http://localhost:8082/wallets/$($wallet.id)" `
    -Headers @{ Authorization = "Bearer $walletToken" }
if ($finalWallet.balance.amount -ne "20.00") {
    throw "Expected final balance 20.00, got $($finalWallet.balance.amount)"
}

$ledger = Invoke-RestMethod -Method Get -Uri "http://localhost:8082/wallets/$($wallet.id)/ledger?limit=50" `
    -Headers @{ Authorization = "Bearer $walletToken" }
$debits = @($ledger.items | Where-Object { $_.direction -eq "DEBIT" })
if ($debits.Count -ne 1) {
    throw "Expected exactly one debit ledger entry, got $($debits.Count)"
}

Write-Host "MULTI-INSTANCE PASS: 3 processes, one BET processed, one rejected, final balance 20.00, one debit."
