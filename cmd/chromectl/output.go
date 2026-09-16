package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/session"
)

func printResult(cmd *cobra.Command, flags *globalFlags, v any, human string) error {
	out := cmd.OutOrStdout()
	if flags.json {
		return json.NewEncoder(out).Encode(v)
	}
	_, err := fmt.Fprintln(out, human)
	return err
}

func dialogLines(events []session.DialogEvent) string {
	var b strings.Builder
	for _, d := range events {
		fmt.Fprintf(&b, "диалог %s: %s → %s\n", d.Type, d.Message, d.Action)
	}
	return b.String()
}
