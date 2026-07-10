[CmdletBinding()]
param(
  [string]$ServerUrl = "",
  [int]$ServerId = 0,
  [string]$SecretKey = "",
  [string]$Repo = "EnderKC/BetterMonitor",
  [string]$Version = "",
  [ValidateSet("stable", "prerelease", "nightly")][string]$Channel = "stable",
  [ValidateSet("full", "monitor")][string]$AgentType = "full",
  [ValidateSet("debug", "info", "warn", "error")][string]$LogLevel = "info",
  [string]$InstallDir = "",
  [string]$GitHubToken = "",
  [string]$ContractTestFixture = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$ServiceName = "BetterMonitorAgent"

function Write-Info([string]$Message) { Write-Host "[install] $Message" }
function Fail([string]$Message) { throw $Message }

function ConvertTo-StrictSemVer([string]$Raw) {
  $match = [regex]::Match(
    $Raw.Trim(),
    '^(?:v)?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$'
  )
  if (-not $match.Success) { return $null }
  $pre = @()
  if ($match.Groups[4].Success) { $pre = @($match.Groups[4].Value -split '\.') }
  foreach ($identifier in $pre) {
    if ($identifier -match '^[0-9]+$' -and $identifier.Length -gt 1 -and $identifier.StartsWith('0')) {
      return $null
    }
  }
  return [pscustomobject]@{
    Major = [uint64]$match.Groups[1].Value
    Minor = [uint64]$match.Groups[2].Value
    Patch = [uint64]$match.Groups[3].Value
    Pre = $pre
    Text = $Raw.Trim().TrimStart('v')
  }
}

function Compare-StrictSemVer([object]$Left, [object]$Right) {
  foreach ($field in @('Major', 'Minor', 'Patch')) {
    if ($Left.$field -gt $Right.$field) { return 1 }
    if ($Left.$field -lt $Right.$field) { return -1 }
  }
  if ($Left.Pre.Count -eq 0 -and $Right.Pre.Count -eq 0) { return 0 }
  if ($Left.Pre.Count -eq 0) { return 1 }
  if ($Right.Pre.Count -eq 0) { return -1 }
  $limit = [Math]::Min($Left.Pre.Count, $Right.Pre.Count)
  for ($index = 0; $index -lt $limit; $index++) {
    $leftId = $Left.Pre[$index]
    $rightId = $Right.Pre[$index]
    if ($leftId -eq $rightId) { continue }
    $leftNumeric = $leftId -match '^[0-9]+$'
    $rightNumeric = $rightId -match '^[0-9]+$'
    if ($leftNumeric -and $rightNumeric) {
      if ([uint64]$leftId -gt [uint64]$rightId) { return 1 }
      return -1
    }
    if ($leftNumeric -ne $rightNumeric) {
      if ($leftNumeric) { return -1 }
      return 1
    }
    $comparison = [string]::CompareOrdinal($leftId, $rightId)
    if ($comparison -gt 0) { return 1 }
    return -1
  }
  if ($Left.Pre.Count -gt $Right.Pre.Count) { return 1 }
  if ($Left.Pre.Count -lt $Right.Pre.Count) { return -1 }
  return 0
}

function Get-StrictReleaseChannel([object]$Release, [object]$ParsedVersion) {
  $nightly = $false
  foreach ($identifier in $ParsedVersion.Pre) {
    if ($identifier.ToLowerInvariant().Contains('nightly')) { $nightly = $true }
  }
  if ($ParsedVersion.Pre.Count -eq 0 -and -not [bool]$Release.prerelease) { return 'stable' }
  if ($ParsedVersion.Pre.Count -gt 0 -and [bool]$Release.prerelease -and $nightly) { return 'nightly' }
  if ($ParsedVersion.Pre.Count -gt 0 -and [bool]$Release.prerelease -and -not $nightly) { return 'prerelease' }
  return 'invalid'
}

function Get-NormalizedArch([string]$Arch) {
  switch ($Arch.ToLowerInvariant()) {
    { $_ -in @('amd64', 'x86_64') } { return 'amd64' }
    { $_ -in @('arm64', 'aarch64') } { return 'arm64' }
    { $_ -in @('386', 'i386', 'i686', 'x86') } { return '386' }
    default { return $Arch.ToLowerInvariant() }
  }
}

function Get-CanonicalAgentAssetName([string]$VersionText, [string]$OS, [string]$Arch, [string]$Type) {
  $base = 'better-monitor-agent'
  if ($Type -eq 'monitor') { $base = 'better-monitor-agent-monitor' }
  $name = "$base-$VersionText-$OS-$Arch"
  if ($OS -eq 'windows') { $name += '.exe' }
  return $name
}

function Find-ExactAsset([object]$Release, [string]$Name) {
  foreach ($asset in @($Release.assets)) {
    if ($asset.name -eq $Name) { return $asset }
  }
  return $null
}

function Resolve-AgentReleaseContract(
  [object[]]$Releases,
  [string]$RequestedChannel,
  [string]$TargetVersion,
  [string]$OS,
  [string]$Arch,
  [string]$Type
) {
  $entries = @()
  foreach ($release in $Releases) {
    if ([bool]$release.draft) { continue }
    $parsed = ConvertTo-StrictSemVer $release.tag_name.ToString()
    if ($null -eq $parsed) { continue }
    $entries += [pscustomobject]@{
      Release = $release
      Version = $parsed
      Channel = Get-StrictReleaseChannel -Release $release -ParsedVersion $parsed
    }
  }

  $selected = $null
  if ($TargetVersion.Trim() -ne '') {
    $target = ConvertTo-StrictSemVer $TargetVersion
    if ($null -eq $target) { Fail 'release_version_invalid' }
    foreach ($entry in $entries) {
      if ($entry.Version.Text -eq $target.Text) { $selected = $entry; break }
    }
    if ($null -eq $selected) { Fail 'release_not_found' }
    if ($selected.Channel -ne $RequestedChannel) { Fail 'release_channel_mismatch' }
  } else {
    foreach ($entry in $entries) {
      if ($entry.Channel -ne $RequestedChannel) { continue }
      if ($null -eq $selected -or (Compare-StrictSemVer $entry.Version $selected.Version) -gt 0) {
        $selected = $entry
      }
    }
    if ($null -eq $selected) { Fail 'release_not_found' }
  }

  $normalizedArch = Get-NormalizedArch $Arch
  $assetName = Get-CanonicalAgentAssetName $selected.Version.Text $OS $normalizedArch $Type
  $asset = Find-ExactAsset $selected.Release $assetName
  if ($null -eq $asset) { Fail 'release_asset_missing' }
  $checksum = Find-ExactAsset $selected.Release 'SHA256SUMS'
  if ($null -eq $checksum) { Fail 'release_checksum_missing' }
  foreach ($item in @($asset, $checksum)) {
    $uri = $null
    if (-not [Uri]::TryCreate($item.browser_download_url.ToString(), [UriKind]::Absolute, [ref]$uri) -or $uri.Scheme -ne 'https') {
      Fail 'release_asset_invalid'
    }
    if ([int64]$item.size -le 0) { Fail 'release_asset_invalid' }
  }
  return [pscustomobject]@{
    Release = $selected.Release
    Tag = $selected.Release.tag_name.ToString()
    Version = $selected.Version.Text
    AssetName = $assetName
    Asset = $asset
    ChecksumAsset = $checksum
  }
}

function Invoke-ContractTest([string]$FixturePath) {
  $fixture = Get-Content -Raw -Path $FixturePath | ConvertFrom-Json
  foreach ($case in @($fixture.cases)) {
    $errorProperty = $case.PSObject.Properties['expected_error']
    $expectedError = if ($null -eq $errorProperty) { '' } else { $errorProperty.Value.ToString() }
    try {
      $resolved = Resolve-AgentReleaseContract `
        -Releases @($fixture.releases) `
        -RequestedChannel $case.channel.ToString() `
        -TargetVersion $case.target_version.ToString() `
        -OS $case.os.ToString() `
        -Arch $case.arch.ToString() `
        -Type $case.agent_type.ToString()
      if ($expectedError -ne '') { Fail "contract case $($case.name) expected $expectedError" }
      if ($resolved.Tag -ne $case.expected_tag.ToString()) { Fail "contract case $($case.name) tag mismatch" }
      if ($resolved.AssetName -ne $case.expected_asset.ToString()) { Fail "contract case $($case.name) asset mismatch" }
      $checksumProperty = $resolved.Release.checksums.PSObject.Properties[$resolved.AssetName]
      $actualChecksum = if ($null -eq $checksumProperty) { '' } else { $checksumProperty.Value.ToString() }
      if ($actualChecksum -ne $case.expected_sha256.ToString()) { Fail "contract case $($case.name) checksum mismatch" }
    } catch {
      if ($expectedError -eq '' -or $_.Exception.Message -ne $expectedError) { throw }
    }
  }
  Write-Info 'Agent release contract fixture passed'
}

function Invoke-GitHubApi([string]$Uri) {
  $headers = @{ Accept = 'application/vnd.github+json'; 'User-Agent' = 'better-monitor-agent-installer' }
  if ($GitHubToken -ne '') { $headers.Authorization = "Bearer $GitHubToken" }
  return Invoke-RestMethod -Uri $Uri -Headers $headers -Method Get
}

function Download-File([string]$Uri, [string]$Path) {
  $headers = @{ 'User-Agent' = 'better-monitor-agent-installer' }
  if ($GitHubToken -ne '') { $headers.Authorization = "Bearer $GitHubToken" }
  Invoke-WebRequest -Uri $Uri -OutFile $Path -Headers $headers -UseBasicParsing
}

function Get-ExpectedChecksum([string]$ChecksumPath, [string]$AssetName) {
  foreach ($line in Get-Content -Path $ChecksumPath) {
    if ($line -match '^(?<hash>[0-9a-fA-F]{64})\s+\*?(?<file>.+)$') {
      $file = $Matches.file.Trim()
      if ($file.StartsWith('./')) { $file = $file.Substring(2) }
      if ($file -eq $AssetName) { return $Matches.hash.ToLowerInvariant() }
    }
  }
  Fail "SHA256SUMS missing entry for $AssetName"
}

function Test-IsAdmin {
  $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
  $principal = New-Object Security.Principal.WindowsPrincipal($identity)
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Start-AgentScheduledTask([string]$TaskName) {
  Start-ScheduledTask -TaskName $TaskName -ErrorAction Stop
  for ($attempt = 0; $attempt -lt 30; $attempt++) {
    $task = Get-ScheduledTask -TaskName $TaskName -ErrorAction Stop
    $taskInfo = Get-ScheduledTaskInfo -TaskName $TaskName -ErrorAction Stop
    if ($task.State -eq 'Running') { return }
    if ($taskInfo.LastTaskResult -ne 0 -and $taskInfo.LastTaskResult -ne 267009) {
      Fail "Scheduled Task exited with result $($taskInfo.LastTaskResult)"
    }
    Start-Sleep -Seconds 1
  }
  Fail 'Scheduled Task did not enter Running state'
}

if ($ContractTestFixture -ne '') {
  Invoke-ContractTest $ContractTestFixture
  exit 0
}

if ($ServerUrl.Trim() -eq '' -or $ServerId -le 0 -or $SecretKey.Trim() -eq '') {
  Fail 'ServerUrl, ServerId and SecretKey are required'
}
if ($null -eq (ConvertTo-StrictSemVer $(if ($Version -eq '') { '0.0.0' } else { $Version }))) {
  Fail 'Version must be strict SemVer when specified'
}

try { [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12 } catch {}
$architecture = Get-NormalizedArch $env:PROCESSOR_ARCHITECTURE
$releases = @(Invoke-GitHubApi "https://api.github.com/repos/$Repo/releases?per_page=100")
$resolved = Resolve-AgentReleaseContract `
  -Releases $releases `
  -RequestedChannel $Channel `
  -TargetVersion $Version `
  -OS 'windows' `
  -Arch $architecture `
  -Type $AgentType

if ($InstallDir -eq '') {
  if (Test-IsAdmin) { $InstallDir = Join-Path $env:ProgramFiles 'BetterMonitor\Agent' }
  else { $InstallDir = Join-Path $env:LOCALAPPDATA 'BetterMonitor\Agent' }
}
[System.IO.Directory]::CreateDirectory($InstallDir) | Out-Null
$agentExe = Join-Path $InstallDir 'better-monitor-agent.exe'
$backupExe = "$agentExe.old"
$configPath = Join-Path $InstallDir 'agent.yaml'
$downloadPath = Join-Path ([System.IO.Path]::GetTempPath()) ("bm-agent-" + [Guid]::NewGuid().ToString('n') + '.exe')
$checksumPath = "$downloadPath-SHA256SUMS"

Download-File $resolved.Asset.browser_download_url.ToString() $downloadPath
Download-File $resolved.ChecksumAsset.browser_download_url.ToString() $checksumPath
$expected = Get-ExpectedChecksum $checksumPath $resolved.AssetName
$actual = (Get-FileHash -Path $downloadPath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { Fail 'SHA256 verification failed' }

$existingTask = Get-ScheduledTask -TaskName $ServiceName -ErrorAction Ignore
$hadExisting = Test-Path $agentExe
if ($null -ne $existingTask) { Stop-ScheduledTask -TaskName $ServiceName -ErrorAction Stop }
if ($hadExisting) { Copy-Item -Force $agentExe $backupExe -ErrorAction Stop }

try {
  Move-Item -Force $downloadPath $agentExe -ErrorAction Stop
  $yaml = @"
server_url: '$ServerUrl'
server_id: $ServerId
secret_key: '$SecretKey'
heartbeat_interval: '10s'
monitor_interval: '30s'
log_level: '$LogLevel'
log_file: '$(Join-Path $InstallDir 'agent.log')'
agent_type: '$AgentType'
"@
  [System.IO.File]::WriteAllText($configPath, $yaml, [System.Text.UTF8Encoding]::new($false))
  $arguments = "--config `"$configPath`""
  $action = New-ScheduledTaskAction -Execute $agentExe -Argument $arguments -WorkingDirectory $InstallDir
  $trigger = if (Test-IsAdmin) { New-ScheduledTaskTrigger -AtStartup } else { New-ScheduledTaskTrigger -AtLogOn }
  $settings = New-ScheduledTaskSettingsSet -StartWhenAvailable
  if (Test-IsAdmin) {
    $principal = New-ScheduledTaskPrincipal -UserId 'SYSTEM' -LogonType ServiceAccount -RunLevel Highest
    Register-ScheduledTask -TaskName $ServiceName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null
  } else {
    Register-ScheduledTask -TaskName $ServiceName -Action $action -Trigger $trigger -Settings $settings -Force | Out-Null
  }
  Start-AgentScheduledTask -TaskName $ServiceName
} catch {
  if ($hadExisting -and (Test-Path $backupExe)) {
    Copy-Item -Force $backupExe $agentExe -ErrorAction Stop
    if ($null -ne $existingTask) { Start-AgentScheduledTask -TaskName $ServiceName }
  }
  throw
} finally {
  Remove-Item -Force $checksumPath -ErrorAction Ignore
  Remove-Item -Force $downloadPath -ErrorAction Ignore
}

Write-Info "Installed $($resolved.AssetName)"
