$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root "install-agent.ps1") -ContractTestFixture (Join-Path $root "testdata/agent-release-contract.json")
