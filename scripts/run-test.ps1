$ErrorActionPreference = "Continue"
Set-Location (Split-Path -Parent $PSScriptRoot)

foreach ($binary in @("Setup.exe", "ProvisionTool.exe")) {
    if (-not (Test-Path $binary)) { continue }
    Write-Output "Trying $binary"
    try {
        $output = & ".\$binary" 2>&1 | Out-String
        Write-Output $output
        Write-Output ("EXIT=" + $LASTEXITCODE)
    } catch {
        Write-Output ("FAILED: " + $_.Exception.Message)
    }
    Write-Output ""
}
