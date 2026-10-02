$ErrorActionPreference = "Stop"

$failures = @()

function Run-Step($Name, $Script) {
    Write-Host ""
    Write-Host "=== $Name ==="
    & powershell -NoProfile -ExecutionPolicy Bypass -File $Script
    if ($LASTEXITCODE -ne 0) {
        $script:failures += "$Name (exit $LASTEXITCODE)"
        Write-Host "STEP FAILED: $Name" -ForegroundColor Red
        $script:lastStepSucceeded = $false
        return
    }
    $script:lastStepSucceeded = $true
}

Write-Host "Resetting containers while preserving no stale test state..."
docker compose --profile multi down -v --remove-orphans
if ($LASTEXITCODE -ne 0) { throw "docker compose down failed" }

# Foundation gates are prerequisites: if either fails, later scenario results are
# not trustworthy, so stop immediately.
Run-Step "Go/integration verification" "scripts/verify.ps1"
if (-not $lastStepSucceeded) { exit 1 }
Run-Step "Authenticated HTTP smoke" "scripts/smoke.ps1"
if (-not $lastStepSucceeded) { exit 1 }

# Scenario gates are independent enough to run to completion. This lets one
# full-verify invocation surface every remaining problem instead of failing fast.
Run-Step "Three-process concurrency" "scripts/multi-instance.ps1"

# Recovery must prove that restarting the primary process is sufficient; ensure
# replicas from the concurrency scenario cannot consume work in later scenarios.
docker compose --profile multi stop api-2 api-3 | Out-Null
if ($LASTEXITCODE -ne 0) {
    $failures += "stop extra API replicas (exit $LASTEXITCODE)"
}

Run-Step "Cross-channel SQS -> HTTP idempotency" "scripts/cross-channel.ps1"
Run-Step "SQS retry and DLQ" "scripts/dlq.ps1"
Run-Step "Restart / pending-reference / outbox recovery" "scripts/recovery.ps1"

Write-Host ""
if ($failures.Count -gt 0) {
    Write-Host "FULL VERIFY FAILED:" -ForegroundColor Red
    foreach ($failure in $failures) { Write-Host " - $failure" -ForegroundColor Red }
    exit 1
}

Write-Host "FULL VERIFY PASS" -ForegroundColor Green
