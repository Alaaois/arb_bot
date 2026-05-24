param(
    [string]$BaseUrl = "http://127.0.0.1:8080",
    [string]$OutputDir = ".\stats",
    [int]$IntervalMinutes = 5,
    [int]$DurationDays = 7
)

$ErrorActionPreference = "Stop"

$endpoints = @(
    "health",
    "funding",
    "opportunities",
    "positions",
    "pnl"
)

$historyEndpoints = @(
    @{ Name = "positions-history"; Path = "positions/history?limit=200" }
)

$startTime = Get-Date
$endTime = $startTime.AddDays($DurationDays)

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $OutputDir "logs") | Out-Null

foreach ($endpoint in $endpoints) {
    New-Item -ItemType Directory -Force -Path (Join-Path $OutputDir $endpoint) | Out-Null
}

foreach ($item in $historyEndpoints) {
    New-Item -ItemType Directory -Force -Path (Join-Path $OutputDir $item.Name) | Out-Null
}

$logFile = Join-Path $OutputDir "logs\collector.log"

function Write-CollectorLog {
    param([string]$Message)
    $line = "{0} {1}" -f (Get-Date -Format "yyyy-MM-ddTHH:mm:ssK"), $Message
    Add-Content -Path $logFile -Value $line
    Write-Host $line
}

function Save-EndpointSnapshot {
    param(
        [string]$Name,
        [string]$Url
    )

    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $targetDir = Join-Path $OutputDir $Name
    $targetFile = Join-Path $targetDir "$timestamp.json"

    try {
        $response = Invoke-RestMethod -Uri $Url -Method Get -TimeoutSec 20
        $json = $response | ConvertTo-Json -Depth 20
        Set-Content -Path $targetFile -Value $json -Encoding UTF8
        Write-CollectorLog "saved $Name -> $targetFile"
    }
    catch {
        Write-CollectorLog "failed $Name -> $($_.Exception.Message)"
    }
}

Write-CollectorLog "collector started; base_url=$BaseUrl output_dir=$OutputDir interval_minutes=$IntervalMinutes duration_days=$DurationDays"

while ((Get-Date) -lt $endTime) {
    foreach ($endpoint in $endpoints) {
        Save-EndpointSnapshot -Name $endpoint -Url "$BaseUrl/$endpoint"
    }

    foreach ($item in $historyEndpoints) {
        Save-EndpointSnapshot -Name $item.Name -Url "$BaseUrl/$($item.Path)"
    }

    Start-Sleep -Seconds ($IntervalMinutes * 60)
}

Write-CollectorLog "collector finished"
