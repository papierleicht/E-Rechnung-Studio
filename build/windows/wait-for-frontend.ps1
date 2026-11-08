param(
    [ValidateRange(1, 600)]
    [int]$TimeoutSeconds = 120
)

$ErrorActionPreference = 'Stop'
# Wails sets this only for development; ordinary executable runs need no server.
$frontendUrl = $env:FRONTEND_DEVSERVER_URL
if ([string]::IsNullOrWhiteSpace($frontendUrl)) {
    exit 0
}

Write-Host "Waiting for frontend dev server: $frontendUrl"
$timer = [Diagnostics.Stopwatch]::StartNew()
while ($timer.Elapsed.TotalSeconds -lt $TimeoutSeconds) {
    try {
        $response = Invoke-WebRequest -Uri $frontendUrl -UseBasicParsing -TimeoutSec 5
        if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 400) {
            exit 0
        }
    } catch {
        # npm ci and Vite may take longer than Wails' built-in startup grace period.
    }
    Start-Sleep -Milliseconds 250
}

Write-Error "Frontend dev server did not become ready within $TimeoutSeconds seconds: $frontendUrl. Check the frontend task output."
exit 1
