param(
    [string]$Version = "dev-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$Package = "aowugong-go-$Version-linux-amd64"
$Dist = Join-Path $Root "dist"
$PackageDir = Join-Path $Dist $Package

Push-Location $Root
try {
    Push-Location "web"
    try {
        npm ci
        npm test
        npm run build
    }
    finally {
        Pop-Location
    }

    go test ./...
    go vet ./...

    Remove-Item -LiteralPath $PackageDir -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Force (Join-Path $PackageDir "web") | Out-Null
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -trimpath -ldflags "-s -w" -o (Join-Path $PackageDir "aowugong") ./cmd/aowugong
    Copy-Item "web/dist" (Join-Path $PackageDir "web/dist") -Recurse
    Copy-Item "migrations", "configs", "init", "scripts" $PackageDir -Recurse
    Copy-Item "README.md" $PackageDir
    [IO.File]::WriteAllText((Join-Path $PackageDir "VERSION"), "$Version`n")

    $Archive = Join-Path $Dist "$Package.tar.gz"
    Remove-Item -LiteralPath $Archive -Force -ErrorAction SilentlyContinue
    tar -C $Dist -czf $Archive $Package
    $Hash = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText("$Archive.sha256", "$Hash  $Package.tar.gz`n")
    Write-Host "Release artifact: $Archive"
}
finally {
    Pop-Location
}
