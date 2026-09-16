package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/pages"
	"github.com/Y91R/chromectl/internal/command/session"
)

const (
	pagesTimeout   = 15 * time.Second
	newPageTimeout = 30 * time.Second
)

func newPagesCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pages",
		Short: "Вкладки: список, открыть, выбрать, закрыть",
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "Список вкладок, * — выбранная",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), pagesTimeout)
			defer cancel()
			list, err := pages.List(ctx, env)
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, p := range list {
				mark := " "
				if p.Selected {
					mark = "*"
				}
				fmt.Fprintf(&b, "%s %s\t%s\t%s\n", mark, p.ID, p.Title, p.URL)
			}
			return printResult(cmd, flags, list, strings.TrimRight(b.String(), "\n"))
		},
	}

	var background bool
	newPage := &cobra.Command{
		Use:   "new <url>",
		Short: "Открыть вкладку и сделать её выбранной",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), newPageTimeout)
			defer cancel()
			page, dialogs, err := pages.New(ctx, env, args[0], background, session.DialogFlag{})
			if err != nil {
				return err
			}
			return printResult(cmd, flags, page, dialogLines(dialogs)+fmt.Sprintf("открыта вкладка %s: %s", page.ID, page.URL))
		},
	}
	newPage.Flags().BoolVar(&background, "background", false, "не выводить вкладку на передний план")

	selectPage := &cobra.Command{
		Use:   "select <id>",
		Short: "Сделать вкладку выбранной",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), pagesTimeout)
			defer cancel()
			page, err := pages.Select(ctx, env, args[0])
			if err != nil {
				return err
			}
			return printResult(cmd, flags, page, fmt.Sprintf("выбрана вкладка %s: %s", page.ID, page.URL))
		},
	}

	closePage := &cobra.Command{
		Use:   "close <id>",
		Short: "Закрыть вкладку (последнюю закрыть нельзя)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), pagesTimeout)
			defer cancel()
			if err := pages.Close(ctx, env, args[0]); err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"closed": args[0]}, "закрыта вкладка "+args[0])
		},
	}

	cmd.AddCommand(list, newPage, selectPage, closePage)
	return cmd
}
