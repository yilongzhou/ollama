#!powershell
#
# powershell -ExecutionPolicy Bypass -File .\scripts\build_windows.ps1

$ErrorActionPreference = "Stop"

function checkEnv() {
    write-host "Locating required tools and paths"
    $script:SRC_DIR=$PWD
    if (!$env:VCToolsRedistDir) {
        $MSVC_INSTALL=(Get-CimInstance MSFT_VSInstance -Namespace root/cimv2/vs)[0].InstallLocation
        $env:VCToolsRedistDir=(get-item "${MSVC_INSTALL}\VC\Redist\MSVC\*")[0]
    }
    $script:NVIDIA_DIR=(get-item "C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v*\bin\")[0]
    $script:INNO_SETUP_DIR=(get-item "C:\Program Files*\Inno Setup*\")[0]

    $script:DEPS_DIR="${script:SRC_DIR}\dist\windeps"
    $env:CGO_ENABLED="1"
    echo "Checking version"
    if (!$env:VERSION) {
        $data=(git describe --tags --first-parent --abbrev=7 --long --dirty --always)
        $pattern="v(.+)"
        if ($data -match $pattern) {
            $script:VERSION=$matches[1]
        }
    } else {
        $script:VERSION=$env:VERSION
    }
    $pattern = "(\d+[.]\d+[.]\d+)-(\d+)-"
    if ($script:VERSION -match $pattern) {
        $script:PKG_VERSION=$matches[1] + "." + $matches[2]
    } else {
        $script:PKG_VERSION=$script:VERSION
    }
    write-host "Building Ollama $script:VERSION with package version $script:PKG_VERSION"
}


function buildOllama() {
    write-host "Building ollama CLI"
    & go generate ./...
    if ($LASTEXITCODE -ne 0) { exit($LASTEXITCODE)}
    & go build "-ldflags=-w -s ""-X=github.com/jmorganca/ollama/version.Version=$script:VERSION"" ""-X=github.com/jmorganca/ollama/server.mode=release""" .
    if ($LASTEXITCODE -ne 0) { exit($LASTEXITCODE)}
}

function buildApp() {
    write-host "Building Ollama App"
    cd "${script:SRC_DIR}\app"
    & go build "-ldflags=-H windowsgui -w -s ""-X=github.com/jmorganca/ollama/version.Version=$script:VERSION"" ""-X=github.com/jmorganca/ollama/server.mode=release""" .
    if ($LASTEXITCODE -ne 0) { exit($LASTEXITCODE)}
}

function gatherDependencies() {
    write-host "Gathering runtime dependencies"
    cd "${script:SRC_DIR}"
    rm -ea 0 -recurse -force -path "${script:DEPS_DIR}"
    md "${script:DEPS_DIR}" -ea 0 > $null

    # TODO - this varies based on host build system and MSVC version - drive from dumpbin output
    # currently works for Win11 + MSVC 2022 + Cuda V12
    cp "${env:VCToolsRedistDir}\x64\Microsoft.VC*.CRT\msvcp140.dll" "${script:DEPS_DIR}\"
    cp "${env:VCToolsRedistDir}\x64\Microsoft.VC*.CRT\vcruntime140.dll" "${script:DEPS_DIR}\"
    cp "${env:VCToolsRedistDir}\x64\Microsoft.VC*.CRT\vcruntime140_1.dll" "${script:DEPS_DIR}\"

    cp "${script:NVIDIA_DIR}\cudart64_*.dll" "${script:DEPS_DIR}\"
    cp "${script:NVIDIA_DIR}\cublas64_*.dll" "${script:DEPS_DIR}\"
    # cp "${script:NVIDIA_DIR}\nvcuda.dll" "${script:DEPS_DIR}\"
}

function buildInstaller() {
    write-host "Building Ollama Installer"
    cd "${script:SRC_DIR}\app"
    $env:PKG_VERSION=$script:PKG_VERSION
    & "${script:INNO_SETUP_DIR}\ISCC.exe" .\ollama.iss
    if ($LASTEXITCODE -ne 0) { exit($LASTEXITCODE)}
}

try {
    checkEnv
    buildOllama
    buildApp
    gatherDependencies
    buildInstaller
} catch {
    write-host "Build Failed"
    write-host $_
} finally {
    set-location $script:SRC_DIR
    $env:PKG_VERSION=""
}