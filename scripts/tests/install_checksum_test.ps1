$ErrorActionPreference = 'Stop'

function Get-ExpectedHash {
    param(
        [string]$ChecksumsPath,
        [string]$ArchiveName
    )

    foreach ($line in Get-Content $ChecksumsPath) {
        if ($line -match '^\s*([a-fA-F0-9]{64})\s+\*?(.+?)\s*$') {
            if ($Matches[2] -eq $ArchiveName) {
                return $Matches[1]
            }
        }
    }

    return $null
}

function Assert-Eq {
    param(
        [string]$Expected,
        [string]$Actual,
        [string]$Message
    )

    if ($Expected -ne $Actual) {
        throw "FAIL: $Message`nExpected: $Expected`nActual: $Actual"
    }
}

function Assert-Null {
    param(
        [string]$Actual,
        [string]$Message
    )

    if ($null -ne $Actual) {
        throw "FAIL: $Message`nExpected: <null>`nActual: $Actual"
    }
}

$checksumsPath = Join-Path $env:TEMP "xf-install-checksum-test-$PID.txt"
@"
1111111111111111111111111111111111111111111111111111111111111111  xf-v1.2.3-windows-amd64.zip
2222222222222222222222222222222222222222222222222222222222222222  xf-v1.2.3-windows-amd64.zip.sig
3333333333333333333333333333333333333333333333333333333333333333 *xf-v1.2.3-linux-amd64.tar.gz
"@ | Set-Content -Path $checksumsPath -NoNewline

try {
    $exact = Get-ExpectedHash -ChecksumsPath $checksumsPath -ArchiveName 'xf-v1.2.3-windows-amd64.zip'
    Assert-Eq -Expected '1111111111111111111111111111111111111111111111111111111111111111' -Actual $exact -Message 'exact filename match should select the correct hash'

    $collision = Get-ExpectedHash -ChecksumsPath $checksumsPath -ArchiveName 'xf-v1.2.3-windows-amd64.zip.sig'
    Assert-Eq -Expected '2222222222222222222222222222222222222222222222222222222222222222' -Actual $collision -Message 'substring collision should not affect exact match parsing'

    $starLine = Get-ExpectedHash -ChecksumsPath $checksumsPath -ArchiveName 'xf-v1.2.3-linux-amd64.tar.gz'
    Assert-Eq -Expected '3333333333333333333333333333333333333333333333333333333333333333' -Actual $starLine -Message 'parser should accept optional binary-marker prefix'

    $missing = Get-ExpectedHash -ChecksumsPath $checksumsPath -ArchiveName 'xf-v1.2.3-darwin-arm64.tar.gz'
    Assert-Null -Actual $missing -Message 'missing checksum entry should produce null'

    if (-not [Environment]::Is64BitOperatingSystem) {
        throw '32-bit Windows is not supported by published releases.'
    }

    Write-Output 'install_checksum_test.ps1: PASS'
}
finally {
    Remove-Item -Path $checksumsPath -Force -ErrorAction SilentlyContinue
}
