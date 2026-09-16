package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Версия chromectl",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if flags.json {
				return json.NewEncoder(out).Encode(struct {
					Version string `json:"version"`
				}{version})
			}
			_, err := fmt.Fprintln(out, version)
			return err
		},
	}
}
