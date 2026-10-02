$ErrorActionPreference = "Stop"

function Wait-Keycloak {
    for ($i = 0; $i -lt 90; $i++) {
        try {
            $response = Invoke-WebRequest -UseBasicParsing "http://localhost:8085/realms/jungle/.well-known/openid-configuration" -TimeoutSec 5
            if ($response.StatusCode -eq 200) {
                # The discovery document may be available slightly before the
                # token endpoint is fully warmed up. Exercise the same client-
                # credentials flow used by the integration test before proceeding.
                $tokenResponse = Invoke-RestMethod -Method Post `
                    -Uri "http://localhost:8085/realms/jungle/protocol/openid-connect/token" `
                    -ContentType "application/x-www-form-urlencoded" `
                    -Body @{
                        grant_type = "client_credentials"
                        client_id = "provider-a"
                        client_secret = "provider-a-secret"
                    } `
                    -TimeoutSec 10
                if ($tokenResponse.access_token) { return }
            }
        } catch {}
        Start-Sleep -Seconds 2
    }
    throw "Keycloak did not become ready"
}

function Wait-LocalStack {
    for ($i = 0; $i -lt 60; $i++) {
        docker compose exec -T localstack awslocal sqs get-queue-url --queue-name wager-transactions.fifo *> $null
        if ($LASTEXITCODE -eq 0) {
            docker compose exec -T localstack awslocal sqs get-queue-url --queue-name wager-events.fifo *> $null
            if ($LASTEXITCODE -eq 0) { return }
        }
        Start-Sleep -Seconds 2
    }
    throw "LocalStack SQS queues did not become ready"
}

Write-Host "Starting real integration dependencies..."
docker compose up -d postgres localstack keycloak
if ($LASTEXITCODE -ne 0) { throw "Could not start integration dependencies" }

Wait-Keycloak
Wait-LocalStack

docker compose run --rm migrate
if ($LASTEXITCODE -ne 0) { throw "Migrations failed" }

go test ./... -count=1
if ($LASTEXITCODE -ne 0) { throw "go test failed" }

go test ./... -count=10
if ($LASTEXITCODE -ne 0) { throw "stability tests failed" }

go test -race ./... -count=1
if ($LASTEXITCODE -ne 0) { throw "race tests failed" }

go vet ./...
if ($LASTEXITCODE -ne 0) { throw "go vet failed" }

$unformatted = gofmt -l .
if ($unformatted) { throw "gofmt required:`n$unformatted" }

go mod verify
if ($LASTEXITCODE -ne 0) { throw "go mod verify failed" }

Write-Host "VERIFY PASS"
