package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"meet-to-md/server/internal/codexapp"
)

func main() {
	binary := flag.String("binary", "codex", "Codex CLI executable")
	model := flag.String("model", "gpt-5.6-luna", "Codex model identifier")
	effort := flag.String("effort", "low", "reasoning effort")
	list := flag.Bool("list", false, "list models and reasoning efforts")
	flag.Parse()
	cwd, _ := filepath.Abs(".")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if *list {
		models, err := (codexapp.ProcessRunner{}).ListModels(ctx, *binary)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, m := range models {
			fmt.Printf("%s:", m.Model)
			for _, option := range m.SupportedReasoningEfforts {
				fmt.Printf(" %s", option.ReasoningEffort)
			}
			fmt.Println()
		}
		return
	}
	result, err := (codexapp.ProcessRunner{}).Run(ctx, *binary, *model, *effort, "Reply with exactly: Hello world!", cwd, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(result)
}
