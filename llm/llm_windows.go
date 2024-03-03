//go:generate powershell -ExecutionPolicy Bypass -File ./gen_windows.ps1
package llm

import "embed"

//go:embed build/windows/amd64/*/bin/*
var libEmbed embed.FS
