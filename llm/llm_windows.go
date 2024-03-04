//go:generate powershell -ExecutionPolicy Bypass -File ./scripts/build_windows.ps1
package llm

import "embed"

//go:embed build/windows/amd64/*/bin/Release/*
var libEmbed embed.FS
