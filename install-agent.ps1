[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)][string]$ServerUrl,
  [Parameter(Mandatory = $true)][int]$ServerId,
  [Parameter(Mandatory = $true)][string]$SecretKey,

  [string]$Repo = "EnderKC/BetterMonitor",
  [string]$Version = "latest",
  [string]$Channel = "stable",
  [string]$LogLevel = "info",

  [string]$ServiceName = "BetterMonitorAgent",
  [string]$AssetName = "",
  [string]$DownloadUrl = "",
  [string]$Sha256 = "",
  [switch]$SkipVerify,

  [string]$InstallDir = "",
  [switch]$UseScheduledTask,
  [string]$GitHubToken = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Write-Info([string]$Message) { Write-Host "[install] $Message" }
function Write-Warn([string]$Message) { Write-Warning $Message }
function Fail([string]$Message) { throw $Message }

function Is-Admin {
  $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($currentIdentity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-Arch {
  $arch = $env:PROCESSOR_ARCHITECTURE
  switch ($arch) {
    "AMD64" { return "amd64" }
    "ARM64" { return "arm64" }
    "x86"   { return "386" }
    default { return "amd64" }
  }
}

function Normalize-Tag([string]$v) {
  $v = $v.Trim()
  if ($v -eq "" -or $v -eq "latest") { return "latest" }
  if ($v.StartsWith("v")) { return $v }
  return "v$($v)"
}

function Invoke-GitHubApi([string]$Uri) {
  $headers = @{
    "Accept" = "application/vnd.github+json"
    "User-Agent" = "better-monitor-agent-installer"
  }
  if ($GitHubToken -ne "") {
    $headers["Authorization"] = "Bearer $GitHubToken"
  }
  return Invoke-RestMethod -Uri $Uri -Headers $headers -Method Get
}

function Resolve-Release([string]$repo, [string]$tag) {
  if ($tag -eq "latest") {
    return Invoke-GitHubApi -Uri "https://api.github.com/repos/$repo/releases/latest"
  }
  return Invoke-GitHubApi -Uri "https://api.github.com/repos/$repo/releases/tags/$tag"
}

function Resolve-ReleaseByChannel([string]$repo, [string]$tag, [string]$channel) {
  if ($null -eq $channel) { $channel = "" }
  $channel = $channel.Trim().ToLowerInvariant()
  if ($channel -eq "") { $channel = "stable" }

  if ($tag -ne "latest") {
    return Resolve-Release -repo $repo -tag $tag
  }

  if ($channel -eq "stable") {
    return Resolve-Release -repo $repo -tag "latest"
  }

  $list = Invoke-GitHubApi -Uri "https://api.github.com/repos/$repo/releases?per_page=20"
  foreach ($r in $list) {
    if ($r.draft) { continue }
    switch ($channel) {
      "prerelease" {
        if ($r.prerelease) { return $r }
      }
      "nightly" {
        $tagName = ""
        if ($null -ne $r.tag_name) { $tagName = $r.tag_name.ToString() }
        $relName = ""
        if ($null -ne $r.name) { $relName = $r.name.ToString() }
        $hay = ($tagName + " " + $relName).ToLowerInvariant()
        if ($hay.Contains("nightly")) { return $r }
      }
      default {
        return $r
      }
    }
  }

  if ($list.Count -gt 0) { return $list[0] }
  Fail "No releases found in repo: $repo"
}

function Resolve-DownloadUrl([object]$release, [string]$assetName) {
  foreach ($a in $release.assets) {
    if ($a.name -eq $assetName) { return $a.browser_download_url }
  }
  return ""
}

function Find-Asset([object]$release, [string]$assetName) {
  foreach ($a in $release.assets) {
    if ($a.name -eq $assetName) { return $a }
  }
  return $null
}

function Get-ReleaseVersion([object]$release) {
  $tagName = ""
  if ($null -ne $release.tag_name) { $tagName = $release.tag_name.ToString() }
  if ($tagName.StartsWith("v")) { return $tagName.Substring(1) }
  return $tagName
}

function Resolve-AgentAsset([object]$release, [string]$arch, [string]$assetName) {
  if ($assetName -ne "") {
    $a = Find-Asset -release $release -assetName $assetName
    if ($null -eq $a) { Fail "Asset not found in release: $assetName" }
    return $a
  }

  $ver = Get-ReleaseVersion -release $release
  $candidates = New-Object System.Collections.Generic.List[string]
  if ($ver -ne "") {
    $candidates.Add("better-monitor-agent-$ver-windows-$arch.exe") | Out-Null
    $candidates.Add("better-monitor-agent-$ver-windows-$arch.zip") | Out-Null
  }
  $candidates.Add("better-monitor-agent-windows-$arch.exe") | Out-Null
  $candidates.Add("better-monitor-agent-windows-$arch.zip") | Out-Null

  foreach ($n in $candidates) {
    $a = Find-Asset -release $release -assetName $n
    if ($null -ne $a) { return $a }
  }

  foreach ($a in $release.assets) {
    $n = ""
    if ($null -ne $a.name) { $n = $a.name.ToString() }
    $n = $n.ToLowerInvariant()
    if ($n.Contains("better-monitor-agent") -and $n.Contains("windows-$arch")) {
      if ($n.EndsWith(".exe") -or $n.EndsWith(".zip")) { return $a }
    }
  }

  Fail "No suitable Windows asset found in release (arch=$arch)."
}

function Resolve-ChecksumAsset([object]$release, [string]$assetName) {
  $candidates = @(
    "$assetName.sha256",
    "$assetName.sha256sum",
    "$assetName.sha256.txt",
    "$assetName.sha256sums",
    "SHA256SUMS",
    "sha256sums.txt",
    "checksums.txt",
    "sha256.txt"
  )
  foreach ($n in $candidates) {
    $a = Find-Asset -release $release -assetName $n
    if ($null -ne $a) { return $a }
  }
  return $null
}

function Parse-Sha256FromText([string]$text, [string]$assetName) {
  foreach ($line in ($text -split "`r?`n")) {
    $l = $line.Trim()
    if ($l -eq "" -or $l.StartsWith("#")) { continue }

    # formats:
    #   <hash>  <file>
    #   <hash> *<file>
    #   <hash>
    if ($l -match "^(?<hash>[0-9a-fA-F]{64})\\s+\\*?(?<file>.+)$") {
      $file = $Matches["file"].Trim()
      if ($file.StartsWith("./")) { $file = $file.Substring(2) }
      if ($file -eq $assetName) { return $Matches["hash"] }
    }
    if ($l -match "^(?<hash>[0-9a-fA-F]{64})$") { return $Matches["hash"] }
  }
  return ""
}

function Download-File([string]$Url, [string]$OutFile) {
  Write-Info "Downloading: $Url"
  $headers = @{ "User-Agent" = "better-monitor-agent-installer" }
  if ($GitHubToken -ne "") {
    $headers["Authorization"] = "Bearer $GitHubToken"
  }
  Invoke-WebRequest -Uri $Url -OutFile $OutFile -Headers $headers -UseBasicParsing
}

function Ensure-Directory([string]$Path) {
  if (-not (Test-Path $Path)) { New-Item -ItemType Directory -Path $Path | Out-Null }
}

function Write-EnvFile([string]$EnvPath) {
  @"
# Generated by install-agent.ps1
SERVER_URL="$ServerUrl"
PANEL_URL="$ServerUrl"
SERVER_ID="$ServerId"
SECRET_KEY="$SecretKey"
CHANNEL="$Channel"
"@ | Set-Content -Path $EnvPath -Encoding UTF8
}

function Write-AgentConfig([string]$ConfigPath, [string]$LogPath) {
  $repo = $Repo
  $channel = $Channel
  $lvl = $LogLevel
  @"
server_url: '$ServerUrl'
server_id: $ServerId
secret_key: '$SecretKey'
register_token: ''
monitor_interval: '30s'
log_level: '$lvl'
log_file: '$LogPath'
enable_cpu_monitor: true
enable_mem_monitor: true
enable_disk_monitor: true
enable_network_monitor: true
update_repo: '$repo'
update_channel: '$channel'
update_mirror: ''
"@ | Set-Content -Path $ConfigPath -Encoding UTF8
}

function Install-ScheduledTask([string]$AgentExe, [string]$WorkDir) {
  Write-Info "Installing scheduled task: $ServiceName"

  $configPath = Join-Path $WorkDir "agent.yaml"
  $args = "--config `"$configPath`""
  $action = New-ScheduledTaskAction -Execute $AgentExe -Argument $args -WorkingDirectory $WorkDir

  if (Is-Admin) {
    $trigger = New-ScheduledTaskTrigger -AtStartup
    $principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
    $settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
    Register-ScheduledTask -TaskName $ServiceName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
    Start-ScheduledTask -TaskName $ServiceName
    return
  }

  $trigger = New-ScheduledTaskTrigger -AtLogOn
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable
  Register-ScheduledTask -TaskName $ServiceName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
  Start-ScheduledTask -TaskName $ServiceName
}

# TLS hardening for older Windows/PowerShell
try { [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 } catch {}

$arch = Get-Arch
$tag = Normalize-Tag $Version

if ($InstallDir -eq "") {
  if (Is-Admin) {
    $InstallDir = Join-Path $env:ProgramFiles "BetterMonitor\Agent"
  } else {
    $InstallDir = Join-Path $env:LOCALAPPDATA "BetterMonitor\Agent"
  }
}

$workDir = $InstallDir
$agentExe = Join-Path $InstallDir "better-monitor-agent.exe"
$envFile = Join-Path $InstallDir "agent.env"
$configFile = Join-Path $InstallDir "agent.yaml"
$logFilePath = Join-Path $InstallDir "agent.log"

Ensure-Directory $InstallDir

$downloadUrl = $DownloadUrl
$release = $null

if ($downloadUrl -eq "") {
  $release = Resolve-ReleaseByChannel -repo $Repo -tag $tag -channel $Channel
  $asset = Resolve-AgentAsset -release $release -arch $arch -assetName $AssetName
  $AssetName = $asset.name
  $downloadUrl = $asset.browser_download_url
}

$tmpPath = Join-Path ([System.IO.Path]::GetTempPath()) ("bm-agent-" + [Guid]::NewGuid().ToString("n"))
$tmpDownload = $tmpPath + "-" + $AssetName
Download-File -Url $downloadUrl -OutFile $tmpDownload

# Extract if zip
$tmpExe = $tmpDownload
if ($tmpDownload.ToLowerInvariant().EndsWith(".zip")) {
  $extractDir = $tmpPath + "-extract"
  Ensure-Directory $extractDir
  Expand-Archive -Path $tmpDownload -DestinationPath $extractDir -Force
  $exe = Get-ChildItem -Path $extractDir -Recurse -File | Where-Object { $_.Name.ToLowerInvariant().EndsWith(".exe") } | Select-Object -First 1
  if ($null -eq $exe) { Fail "No .exe found in archive: $AssetName" }
  $tmpExe = $exe.FullName
}

if (-not $SkipVerify) {
  $expected = $Sha256.Trim()
  if ($expected -eq "") {
    if ($null -ne $release -and $AssetName -ne "") {
      $checksumAsset = Resolve-ChecksumAsset -release $release -assetName $AssetName
      if ($null -ne $checksumAsset) {
        $tmpChecksum = $tmpPath + "-" + $checksumAsset.name
        Download-File -Url $checksumAsset.browser_download_url -OutFile $tmpChecksum
        $text = Get-Content -Raw -Path $tmpChecksum
        $expected = Parse-Sha256FromText -text $text -assetName $AssetName
      }
    }
  }

  if ($expected -ne "") {
    $actual = (Get-FileHash -Path $tmpExe -Algorithm SHA256).Hash
    if ($actual.ToLowerInvariant() -ne $expected.Trim().ToLowerInvariant()) {
      Fail "SHA256 mismatch: expected=$expected actual=$actual"
    }
    Write-Info "SHA256 verified"
  } else {
    Write-Warn "No SHA256 provided/found; skipping verification (use -Sha256 <hash> or ensure release has SHA256SUMS)"
  }
}

Move-Item -Force $tmpExe $agentExe

Write-EnvFile -EnvPath $envFile
Write-AgentConfig -ConfigPath $configFile -LogPath $logFilePath

Write-Info "Installed: $agentExe"
Write-Info "Config: $envFile"
Write-Info "Agent config: $configFile"

if ($UseScheduledTask) {
  Install-ScheduledTask -AgentExe $agentExe -WorkDir $workDir
  Write-Info "Done (scheduled task)"
  exit 0
}

if (-not (Is-Admin)) {
  Write-Warn "Not running as Administrator; falling back to scheduled task."
  Install-ScheduledTask -AgentExe $agentExe -WorkDir $workDir
  Write-Info "Done (scheduled task)"
  exit 0
}

Write-Warn "NSSM service installation not implemented. Using scheduled task."
Install-ScheduledTask -AgentExe $agentExe -WorkDir $workDir
Write-Info "Done (scheduled task)"
