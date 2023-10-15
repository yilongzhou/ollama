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
	execPath, _ := os.Executable()
	if strings.HasSuffix(execPath, ".app/Contents/MacOS/Ollama") {
		app.Run()
		return
	}

	cobra.CheckErr(cmd.NewCLI().ExecuteContext(context.Background()))
}
