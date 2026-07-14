# Build Setup.exe for Windows 11 x64
$ErrorActionPreference = "Stop"

$version = if ($env:VERSION) { $env:VERSION } else { "1.0.0" }
$ldflags = "-s -w -X main.version=$version"

$env:GOOS = "windows"
$env:GOARCH = "amd64"

Write-Host "Building Setup.exe (version $version)..."
go build -trimpath -ldflags $ldflags -o Setup.exe ./cmd/setup

if ($LASTEXITCODE -ne 0) {
    Write-Error "Build failed"
    exit 1
}

Write-Host "Build succeeded: Setup.exe"
