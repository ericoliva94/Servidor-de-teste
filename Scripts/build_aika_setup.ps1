$ErrorActionPreference = 'Stop'

$scriptsDir = $PSScriptRoot
$source = Join-Path $scriptsDir 'aika_installer_gui.py'
$workDir = Join-Path $scriptsDir '.build'

if (-not (Test-Path -LiteralPath $source)) {
    throw "Source file not found: $source"
}

py -m pip install --upgrade pyinstaller
py -m PyInstaller --noconfirm --clean --onefile --windowed `
    --name AikaSetupStation `
    --distpath $scriptsDir `
    --workpath $workDir `
    --specpath $scriptsDir `
    $source

Write-Host "Executable created at: $(Join-Path $scriptsDir 'AikaSetupStation.exe')"
