# Focus Lock - trigger a release now (hotfix / testing)
#
# Runs the "release" GitHub Actions workflow (.github/workflows/release.yml) on
# main via workflow_dispatch. A manual run always releases, even with no new
# commits. Needs the GitHub CLI (gh), signed in with access to the repo.
#
#   powershell -ExecutionPolicy Bypass -File scripts\release.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\release.ps1 -Watch

#requires -Version 5
param(
    [switch]$Watch
)
$ErrorActionPreference = 'Stop'

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    throw "GitHub CLI (gh) not found. Install it from https://cli.github.com/ and run 'gh auth login'."
}

& gh workflow run release.yml --ref main
if ($LASTEXITCODE -ne 0) { throw "gh workflow run failed (exit $LASTEXITCODE)" }
Write-Host "[release] Release workflow triggered on main." -ForegroundColor Green

if ($Watch) {
    Start-Sleep -Seconds 5   # give GitHub a moment to register the run
    $id = (& gh run list --workflow release.yml --limit 1 --json databaseId --jq '.[0].databaseId')
    if ($id) { & gh run watch $id --exit-status }
} else {
    Write-Host "Follow it with: gh run list --workflow release.yml" -ForegroundColor Cyan
}
