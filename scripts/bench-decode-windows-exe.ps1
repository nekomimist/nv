param(
    [Parameter(Mandatory = $true)]
    [string]$StdlibExe,
    [Parameter(Mandatory = $true)]
    [string]$NativeExe,
    [int]$Count = 5,
    [string]$Bench = "^BenchmarkDecode",
    [string]$ImageDir = "",
    [switch]$NativeOnly,
    [switch]$StdlibOnly
)

$ErrorActionPreference = "Stop"

function Run-BenchExe {
    param(
        [string]$Name,
        [string]$ExePath
    )

    Write-Host ""
    Write-Host "== $Name =="
    $benchArgs = @(
        "-test.run", "^$",
        "-test.bench", $Bench,
        "-test.benchmem",
        "-test.count", "$Count"
    )
    & $ExePath @benchArgs
    if ($LASTEXITCODE -ne 0) {
        throw "$Name failed with exit code $LASTEXITCODE"
    }
}

if ($ImageDir -ne "") {
    $env:NV_BENCH_IMAGE_DIR = $ImageDir
}

Write-Host "Stdlib exe: $StdlibExe"
Write-Host "Native exe: $NativeExe"
if ($env:NV_BENCH_IMAGE_DIR) {
    Write-Host "NV_BENCH_IMAGE_DIR: $env:NV_BENCH_IMAGE_DIR"
}

if (-not $NativeOnly) {
    Run-BenchExe -Name "Windows stdlib decode" -ExePath $StdlibExe
}

if (-not $StdlibOnly) {
    Run-BenchExe -Name "Windows WIC native decode" -ExePath $NativeExe
}
