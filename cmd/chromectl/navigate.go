package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/pages"
	"github.com/Y91R/chromectl/internal/command/session"
)

type navigateFlags struct {
	back        bool
	forward     bool
	reload      bool
	ignoreCache bool
	initScript  string
	timeout     time.Duration
	dialog      string
	capture     bool
	unredacted  bool
}

func newNavigateCmd(flags *globalFlags) *cobra.Command {
	var nf navigateFlags
	cmd := &cobra.Command{
		Use:   "navigate [url]",
		Short: "Перейти по URL, назад, вперёд или перезагрузить выбранную вкладку",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			opts := pages.NavigateOptions{
				Back:        nf.back,
				Forward:     nf.forward,
				Reload:      nf.reload,
				IgnoreCache: nf.ignoreCache,
				InitScript:  nf.initScript,
				Dialog:      session.DialogFlag{Value: nf.dialog, Set: cmd.Flags().Changed("dialog")},
				Capture:     nf.capture,
				Unredacted:  nf.unredacted,
			}
			if len(args) == 1 {
				opts.URL = args[0]
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), nf.timeout)
			defer cancel()
			res, err := pages.Navigate(ctx, env, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, res, dialogLines(res.Dialogs)+fmt.Sprintf("открыто: %s — %s", res.URL, res.Title))
		},
	}
	cmd.Flags().BoolVar(&nf.back, "back", false, "назад по истории")
	cmd.Flags().BoolVar(&nf.forward, "forward", false, "вперёд по истории")
	cmd.Flags().BoolVar(&nf.reload, "reload", false, "перезагрузить; снимает блокировку диалогом, оставшимся от другой команды")
	cmd.Flags().BoolVar(&nf.ignoreCache, "ignore-cache", false, "перезагрузить без кеша")
	cmd.Flags().StringVar(&nf.initScript, "init-script", "", "JS до скриптов страницы, только для этой навигации")
	cmd.Flags().DurationVar(&nf.timeout, "timeout", 30*time.Second, "ожидание загрузки")
	cmd.Flags().StringVar(&nf.dialog, "dialog", "accept", "ответ на диалог: accept, dismiss или текст для prompt")
	cmd.Flags().BoolVar(&nf.capture, "capture", false, "записать сетевые запросы навигации для network list/get")
	cmd.Flags().BoolVar(&nf.unredacted, "unredacted", false, "с --capture: не маскировать Authorization и cookie")
	return cmd
}
