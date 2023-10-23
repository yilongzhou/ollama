package main

import (
	"context"
	"os"
	"strings"

	"github.com/jmorganca/ollama/app"
	"github.com/jmorganca/ollama/cmd"
	"github.com/spf13/cobra"
)

func main() {
	// if executing in a .app directory on macOS, start the app
	execPath, _ := os.Executable()
	if strings.HasSuffix(execPath, ".app/Contents/MacOS/Ollama") {
		app.Run()
		return
	}

	cobra.CheckErr(cmd.NewCLI().ExecuteContext(context.Background()))
}
