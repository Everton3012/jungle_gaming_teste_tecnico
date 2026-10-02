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

function Queue-Url($Name) {
    $url = docker compose exec -T localstack awslocal sqs get-queue-url `
        --queue-name $Name --query QueueUrl --output text
    if ($LASTEXITCODE -ne 0) { throw "Could not resolve queue $Name" }
    return ($url | Out-String).Trim()
}

Write-Host "Starting stack for DLQ verification..."
$previousVisibility = $env:SQS_CONSUMER_VISIBILITY_TIMEOUT
$env:SQS_CONSUMER_VISIBILITY_TIMEOUT = "1s"
docker compose up -d --build --force-recreate api
if ($LASTEXITCODE -ne 0) { throw "docker compose up failed" }
Wait-Ready "http://localhost:8080"

$sourceUrl = Queue-Url "wager-transactions.fifo"
$dlqUrl = Queue-Url "wager-transactions-dlq.fifo"
$messageId = "dlq-$([guid]::NewGuid().ToString())"

$invalidEnvelope = @{
    messageId = $messageId
    type = "WagerTransactionRequested"
    occurredAt = [DateTime]::UtcNow.ToString("o")
    data = @{
        externalTransactionId = "invalid-$messageId"
        idempotencyKey = "invalid-$messageId"
        playerId = "player-invalid"
        walletId = "wallet-invalid"
        roundId = "round-invalid"
        gameId = "game-invalid"
        kind = "BET"
        money = @{ amount = "1.00"; currency = "BRL" }
    }
} | ConvertTo-Json -Depth 10 -Compress

try {
    # Reduce only the test queue visibility window so five real deliveries reach
    # the configured DLQ quickly. Restore the production value in finally.
    docker compose exec -T localstack awslocal sqs set-queue-attributes `
        --queue-url $sourceUrl --attributes VisibilityTimeout=1 | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not set test visibility timeout" }

    $invalidEnvelopeBase64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($invalidEnvelope))
    docker compose exec -T localstack sh -c "printf '%s' '$invalidEnvelopeBase64' | base64 -d > /tmp/dlq-message.json"
    if ($LASTEXITCODE -ne 0) { throw "Could not stage DLQ test message" }

    docker compose exec -T localstack awslocal sqs send-message `
        --queue-url $sourceUrl `
        --message-body file:///tmp/dlq-message.json `
        --message-group-id "dlq-test" `
        --message-deduplication-id $messageId | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Could not send DLQ test message" }

    $found = $false
    for ($i = 0; $i -lt 60; $i++) {
        $raw = docker compose exec -T localstack awslocal sqs receive-message `
            --queue-url $dlqUrl `
            --max-number-of-messages 10 `
            --wait-time-seconds 1 `
            --visibility-timeout 0 `
            --attribute-names All
        if ($LASTEXITCODE -ne 0) { throw "Could not inspect DLQ" }

        $text = ($raw | Out-String)
        if ($text -match [regex]::Escape($messageId)) {
            $found = $true
            break
        }
        Start-Sleep -Seconds 1
    }

    if (-not $found) {
        $sourceAttrs = docker compose exec -T localstack awslocal sqs get-queue-attributes --queue-url $sourceUrl --attribute-names All --output json
        $dlqAttrs = docker compose exec -T localstack awslocal sqs get-queue-attributes --queue-url $dlqUrl --attribute-names All --output json
        $inbox = docker compose exec -T postgres psql -U jungle -d jungle_gaming -At -F '|' -c "SELECT status, attempts, COALESCE(last_error,'') FROM inbox_messages WHERE consumer_name='wager-transactions' AND message_id='$messageId';"
        Write-Host "DLQ diagnostic - source queue: $((($sourceAttrs | Out-String).Trim()))"
        Write-Host "DLQ diagnostic - DLQ: $((($dlqAttrs | Out-String).Trim()))"
        Write-Host "DLQ diagnostic - inbox: $((($inbox | Out-String).Trim()))"
        docker compose logs --tail=120 api
        throw "Message $messageId did not reach the DLQ after repeated real SQS deliveries"
    }

    Write-Host "DLQ PASS: invalid wager message was redelivered and moved to wager-transactions-dlq.fifo."
}
finally {
    docker compose exec -T localstack awslocal sqs set-queue-attributes `
        --queue-url $sourceUrl --attributes VisibilityTimeout=30 | Out-Null
    if ($null -eq $previousVisibility) {
        Remove-Item Env:SQS_CONSUMER_VISIBILITY_TIMEOUT -ErrorAction SilentlyContinue
    } else {
        $env:SQS_CONSUMER_VISIBILITY_TIMEOUT = $previousVisibility
    }
}
