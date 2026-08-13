<#
.SYNOPSIS
    Install ctdev, or run a one-off diagnostic without installing anything.

.DESCRIPTION
    With no arguments, installs the ctdev binary to %LOCALAPPDATA%\Programs\ctdev
    and adds it to the user PATH.

    With -Diagnose, downloads ctdev to a temporary directory, runs
    'ctdev diagnose', and deletes it. Nothing is installed, no PATH is changed,
    and no elevation is requested — for machines you're only visiting.

    On Windows, ctdev supports the diagnose command only. Everything else is
    Linux and macOS.

.EXAMPLE
    irm https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.ps1 | iex

.EXAMPLE
    # `irm | iex` cannot pass arguments, so wrap the script in a scriptblock:
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/ConnerTechnology/dotfiles/main/install.ps1))) -Diagnose

.EXAMPLE
    & ([scriptblock]::Create((irm .../install.ps1))) -Diagnose -DiagnoseArgs '--deep','--report'
#>
[CmdletBinding()]
param(
    [switch]$Diagnose,
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$DiagnoseArgs = @()
)

$ErrorActionPreference = 'Stop'
$Repo = 'ConnerTechnology/dotfiles'

# In diagnose mode the report owns stdout, so progress goes to the information
# stream instead — that keeps `| Out-File report.txt` clean.
function Write-Progress-Line {
    param([string]$Message)
    if ($Diagnose) { [Console]::Error.WriteLine("==> $Message") }
    else { Write-Host "==> $Message" -ForegroundColor Blue }
}

function Write-Success {
    param([string]$Message)
    if ($Diagnose) { [Console]::Error.WriteLine("[OK] $Message") }
    else { Write-Host "[OK] $Message" -ForegroundColor Green }
}

function Get-CtdevArch {
    # PROCESSOR_ARCHITECTURE reports the *process* architecture, which is x86
    # for a 32-bit shell on a 64-bit machine. OSArchitecture is the machine.
    switch -Wildcard ((Get-CimInstance Win32_OperatingSystem).OSArchitecture) {
        '*ARM*' { return 'arm64' }
        '64*'   { return 'amd64' }
        default { throw "Unsupported architecture. ctdev ships amd64 and arm64 builds only." }
    }
}

function Get-LatestVersion {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    if (-not $release.tag_name) {
        throw "Could not determine the latest release. See https://github.com/$Repo/releases"
    }
    return $release.tag_name
}

$arch = Get-CtdevArch
Write-Progress-Line "Detected platform: windows-$arch"

Write-Progress-Line "Checking latest release..."
$version = Get-LatestVersion
Write-Progress-Line "Latest version: $version"

$assetName = "ctdev-windows-$arch.exe"
$binaryUrl = "https://github.com/$Repo/releases/download/$version/$assetName"
$sumsUrl = "https://github.com/$Repo/releases/download/$version/SHA256SUMS"

# Stage the download in a temp directory either way. In diagnose mode this is
# also where it runs from, and it is removed in the finally block below.
$workDir = Join-Path ([System.IO.Path]::GetTempPath()) ("ctdev-" + [System.Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $workDir -Force | Out-Null

try {
    $binaryPath = Join-Path $workDir 'ctdev.exe'

    Write-Progress-Line "Downloading $assetName..."
    Invoke-WebRequest -Uri $binaryUrl -OutFile $binaryPath -UseBasicParsing

    # Verify against the release checksums, failing closed. Anyone able to
    # tamper with the download could otherwise just break the SUMS fetch to
    # skip verification. CTDEV_SKIP_VERIFY=1 is the explicit escape hatch.
    if ($env:CTDEV_SKIP_VERIFY -eq '1') {
        Write-Progress-Line "CTDEV_SKIP_VERIFY=1 - skipping checksum verification"
    }
    else {
        $sumsPath = Join-Path $workDir 'SHA256SUMS'
        try {
            Invoke-WebRequest -Uri $sumsUrl -OutFile $sumsPath -UseBasicParsing
        }
        catch {
            throw "Could not fetch SHA256SUMS for $version - refusing to install an unverified binary. Set CTDEV_SKIP_VERIFY=1 if this release predates checksums."
        }

        $expected = $null
        foreach ($line in Get-Content $sumsPath) {
            $fields = $line -split '\s+'
            if ($fields.Count -ge 2 -and $fields[-1] -eq $assetName) {
                $expected = $fields[0]
                break
            }
        }
        if (-not $expected) {
            throw "No checksum entry for $assetName in SHA256SUMS - refusing to install."
        }

        $actual = (Get-FileHash -Path $binaryPath -Algorithm SHA256).Hash
        if ($actual -ne $expected.ToUpperInvariant()) {
            throw "Checksum mismatch for ${assetName}.`n  expected: $expected`n  actual:   $actual"
        }
        Write-Progress-Line "Checksum verified"
    }

    if ($Diagnose) {
        Write-Progress-Line "Running diagnose (nothing is installed)..."
        $arguments = @('diagnose') + $DiagnoseArgs
        & $binaryPath @arguments
        exit $LASTEXITCODE
    }

    $installDir = Join-Path $env:LOCALAPPDATA 'Programs\ctdev'
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    $target = Join-Path $installDir 'ctdev.exe'
    Move-Item -Path $binaryPath -Destination $target -Force

    Write-Success "ctdev $version installed to $target"

    # User PATH only: a machine-wide change would need elevation, and this
    # installer never asks for it.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$installDir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$installDir", 'User')
        Write-Success "Added $installDir to your PATH - open a new terminal to pick it up."
    }

    Write-Host ""
    Write-Host "On Windows, ctdev supports 'ctdev diagnose'. Run it to check this machine." -ForegroundColor Yellow
}
finally {
    if (Test-Path $workDir) {
        Remove-Item -Path $workDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
