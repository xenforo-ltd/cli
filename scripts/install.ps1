#Requires -Version 5.1
<#
.SYNOPSIS
    XenForo CLI Installer for Windows

.DESCRIPTION
    Downloads and installs the XenForo CLI tool on Windows.
    Checks for prerequisites (git, docker) and adds the install location to PATH.

.PARAMETER Version
    Specific version to install (e.g., 1.0.0). If not specified, installs latest.

.PARAMETER Binary
    Path to a local binary to install instead of downloading.

.PARAMETER NoModifyPath
    Don't modify the user's PATH environment variable.

.PARAMETER SkipPrereq
    Skip prerequisite checks (not recommended).

.EXAMPLE
    irm https://raw.githubusercontent.com/xenforo-ltd/xf/main/scripts/install.ps1 | iex

.EXAMPLE
    .\install.ps1 -Version 1.0.0

.EXAMPLE
    .\install.ps1 -Binary C:\path\to\xf.exe
#>

param(
    [string]$Version = "",
    [string]$Binary = "",
    [switch]$NoModifyPath,
    [switch]$SkipPrereq,
    [switch]$Help
)

$ErrorActionPreference = "Stop"

$App = "xf"
$Repo = "xenforo-ltd/xf"
$InstallDir = "$env:USERPROFILE\.xf\bin"

$MinGitVersion = [version]"2.25.1"
$MinDockerVersion = [version]"20.10.0"

function Write-Color {
    param(
        [string]$Text,
        [string]$Color = "White"
    )
    Write-Host $Text -ForegroundColor $Color
}

function Show-Usage {
    @"
XenForo CLI Installer for Windows

Usage: install.ps1 [options]

Options:
    -Help               Display this help message
    -Version <version>  Install a specific version (e.g., 1.0.0)
    -Binary <path>      Install from a local binary instead of downloading
    -NoModifyPath       Don't modify the PATH environment variable
    -SkipPrereq         Skip prerequisite checks

Examples:
    irm https://raw.githubusercontent.com/$Repo/main/scripts/install.ps1 | iex
    .\install.ps1 -Version 1.0.0
    .\install.ps1 -Binary C:\path\to\xf.exe
"@
}

function Compare-Version {
    param(
        [version]$Current,
        [version]$Minimum
    )
    return $Current -ge $Minimum
}

function Get-GitVersion {
    try {
        $output = git --version 2>$null
        if ($output -match "git version (\d+\.\d+\.\d+)") {
            return [version]$Matches[1]
        }
    } catch {}
    return $null
}

function Get-DockerVersion {
    try {
        $output = docker --version 2>$null
        if ($output -match "Docker version (\d+\.\d+\.\d+)") {
            return [version]$Matches[1]
        }
    } catch {}
    return $null
}

function Test-DockerCompose {
    try {
        $null = docker compose version 2>$null
        return $LASTEXITCODE -eq 0
    } catch {
        return $false
    }
}

function Test-Prerequisites {
    $hasErrors = $false
    $missingDeps = @()
    $versionErrors = @()

    Write-Host "Checking prerequisites..."

    $gitVersion = Get-GitVersion
    if ($null -eq $gitVersion) {
        Write-Color "  X git not found" "Red"
        $missingDeps += "git"
        $hasErrors = $true
    } elseif (-not (Compare-Version $gitVersion $MinGitVersion)) {
        Write-Color "  X git $gitVersion (requires >= $MinGitVersion)" "Red"
        $versionErrors += "git"
        $hasErrors = $true
    } else {
        Write-Color "  + git $gitVersion" "Green"
    }

    $dockerVersion = Get-DockerVersion
    if ($null -eq $dockerVersion) {
        Write-Color "  X docker not found" "Red"
        $missingDeps += "docker"
        $hasErrors = $true
    } elseif (-not (Compare-Version $dockerVersion $MinDockerVersion)) {
        Write-Color "  X docker $dockerVersion (requires >= $MinDockerVersion)" "Red"
        $versionErrors += "docker"
        $hasErrors = $true
    } else {
        Write-Color "  + docker $dockerVersion" "Green"
        
        if (Test-DockerCompose) {
            try {
                $composeVersion = (docker compose version --short 2>$null) -replace '[^0-9.]', ''
                Write-Color "  + docker compose $composeVersion" "Green"
            } catch {
                Write-Color "  + docker compose" "Green"
            }
        } else {
            Write-Color "  X docker compose not available" "Red"
            $versionErrors += "docker-compose"
            $hasErrors = $true
        }
    }

    if ($hasErrors) {
        Write-Host ""
        Write-Color "Missing or outdated dependencies:" "Red"
        Write-Host ""

        foreach ($dep in $missingDeps) {
            switch ($dep) {
                "git" {
                    Write-Color "Git is required for XenForo CLI workflows." "Yellow"
                    Write-Host "  Download: https://git-scm.com/download/win"
                    Write-Host "  Or via winget: winget install Git.Git"
                    Write-Host ""
                }
                "docker" {
                    Write-Color "Docker is required to run XenForo development environments." "Yellow"
                    Write-Host "  Download: https://docs.docker.com/desktop/install/windows-install/"
                    Write-Host ""
                }
            }
        }

        foreach ($dep in $versionErrors) {
            switch ($dep) {
                "git" {
                    Write-Color "Git version $MinGitVersion or later is required." "Yellow"
                    Write-Host "  Please update Git: https://git-scm.com/download/win"
                    Write-Host ""
                }
                "docker" {
                    Write-Color "Docker version $MinDockerVersion or later is required." "Yellow"
                    Write-Host "  Please update Docker: https://docs.docker.com/desktop/install/windows-install/"
                    Write-Host ""
                }
                "docker-compose" {
                    Write-Color "Docker Compose V2 is required but not available." "Yellow"
                    Write-Host "  Docker Compose V2 is included with Docker Desktop."
                    Write-Host "  Please update Docker: https://docs.docker.com/desktop/install/windows-install/"
                    Write-Host ""
                }
            }
        }

        Write-Host "Please install the missing dependencies and run this script again."
        Write-Host "Or use -SkipPrereq to bypass these checks (not recommended)."
        exit 1
    }

    Write-Host ""
}

function Get-LatestVersion {
    $url = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $response = Invoke-RestMethod -Uri $url -UseBasicParsing
        return $response.tag_name -replace '^v', ''
    } catch {
        Write-Color "Error: Failed to fetch latest version from GitHub" "Red"
        exit 1
    }
}

function Test-ReleaseExists {
    param([string]$Ver)
    
    $url = "https://github.com/$Repo/releases/tag/v$Ver"
    try {
        $null = Invoke-WebRequest -Uri $url -Method Head -UseBasicParsing
        return $true
    } catch {
        return $false
    }
}

function Get-InstalledVersion {
    try {
        $output = & "$InstallDir\$App.exe" version --short 2>$null
        return $output -replace '^v', ''
    } catch {
        return $null
    }
}

function Add-ToPath {
    if ($NoModifyPath) {
        return
    }

    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "User")
    
    if ($currentPath -split ";" -contains $InstallDir) {
        Write-Host "PATH already contains $InstallDir" 
        return
    }

    $newPath = "$InstallDir;$currentPath"
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
    Write-Host "Added $InstallDir to PATH" 
    
    $env:PATH = "$InstallDir;$env:PATH"

    if ($env:GITHUB_ACTIONS -eq "true" -and $env:GITHUB_PATH) {
        Add-Content -Path $env:GITHUB_PATH -Value $InstallDir
        Write-Host "Added to `$GITHUB_PATH" 
    }
}

function Install-FromBinary {
    param([string]$BinaryPath)

    if (-not (Test-Path $BinaryPath)) {
        Write-Color "Error: Binary not found at $BinaryPath" "Red"
        exit 1
    }

    Write-Host "Installing $App from local binary..."
    
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    
    Copy-Item -Path $BinaryPath -Destination "$InstallDir\$App.exe" -Force

    Add-ToPath

    Write-Host ""
    Write-Color "Successfully installed $App" "Green"
    Write-Host ""
    Write-Host "To get started:"
    Write-Host "  1. Restart your terminal"
    Write-Host "  2. Run: xf auth login"
    Write-Host "  3. Run: xf init .\my-project"
    Write-Host ""
}

function Install-FromGitHub {
    param([string]$Ver)

    if (-not [Environment]::Is64BitOperatingSystem) {
        Write-Color "Error: 32-bit Windows is not supported by published releases." "Red"
        exit 1
    }
    $arch = "amd64"
    
    Write-Host "Detected platform: windows/$arch" 
    Write-Host "Installing $App version v$Ver" -ForegroundColor White

    $installedVersion = Get-InstalledVersion
    if ($installedVersion -eq $Ver) {
        Write-Host "Version $Ver is already installed" 
        exit 0
    }
    if ($installedVersion) {
        Write-Host "Installed version: $installedVersion" 
    }

    $tempDir = Join-Path $env:TEMP "xf-install-$(Get-Random)"
    New-Item -ItemType Directory -Force -Path $tempDir | Out-Null

    try {
        $archiveName = "xf-v$Ver-windows-$arch.zip"
        $downloadUrl = "https://github.com/$Repo/releases/download/v$Ver/$archiveName"
        $checksumsUrl = "https://github.com/$Repo/releases/download/v$Ver/checksums.txt"

        $archivePath = Join-Path $tempDir $archiveName
        $checksumsPath = Join-Path $tempDir "checksums.txt"

        Write-Host "Downloading $archiveName..." 
        Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing

        Write-Host "Downloading checksums..." 
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing

        Write-Host "Verifying checksum..." 
        $expectedHash = $null
        foreach ($line in Get-Content $checksumsPath) {
            if ($line -match '^\s*([a-fA-F0-9]{64})\s+\*?(.+?)\s*$') {
                $entryName = $Matches[2]
                if ($entryName -eq $archiveName) {
                    $expectedHash = $Matches[1]
                    break
                }
            }
        }

        if (-not $expectedHash) {
            Write-Color "Error: No checksum entry found for $archiveName" "Red"
            exit 1
        }

        $actualHash = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
        if ($actualHash -ne $expectedHash.ToLower()) {
            Write-Color "Error: Checksum verification failed" "Red"
            Write-Host "  Expected: $expectedHash"
            Write-Host "  Actual:   $actualHash"
            exit 1
        }
        Write-Color "  + Checksum verified" "Green"

        Write-Host "Extracting..." 
        Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force

        New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

        $binaryPath = Join-Path $tempDir "$App.exe"
        Move-Item -Path $binaryPath -Destination "$InstallDir\$App.exe" -Force

        Add-ToPath

        Write-Host ""
        Write-Color "Successfully installed $App v$Ver" "Green"
        Write-Host ""
        Write-Host "To get started:"
        Write-Host "  1. Restart your terminal"
        Write-Host "  2. Run: xf auth login"
        Write-Host "  3. Run: xf init .\my-project"
        Write-Host ""

    } finally {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

if ($Help) {
    Show-Usage
    exit 0
}

if (-not $SkipPrereq -and -not $Binary) {
    Test-Prerequisites
}

if ($Binary) {
    Install-FromBinary -BinaryPath $Binary
} else {
    if ($Version) {
        $Version = $Version -replace '^v', ''
        if (-not (Test-ReleaseExists $Version)) {
            Write-Color "Error: Release v$Version not found" "Red"
            Write-Host "Available releases: https://github.com/$Repo/releases" 
            exit 1
        }
    } else {
        Write-Host "Fetching latest version..."
        $Version = Get-LatestVersion
    }

    Install-FromGitHub -Ver $Version
}
