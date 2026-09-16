package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/browser"
	"github.com/Y91R/chromectl/internal/command/pages"
)

const (
	startTimeout  = 60 * time.Second
	stopTimeout   = 30 * time.Second
	statusTimeout = 10 * time.Second
)

func newBrowserCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "browser",
		Short: "Запуск и остановка Chrome с debug-портом",
	}

	var opts pages.StartOptions
	start := &cobra.Command{
		Use:   "start",
		Short: "Запустить Chrome с debug-портом --port",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			opts.ChromePath, err = browser.FindChrome(os.Getenv("CHROME_PATH"), browser.DefaultCandidates(runtime.GOOS), browser.FileExists)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), startTimeout)
			defer cancel()
			info, err := pages.Start(ctx, env, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, info, fmt.Sprintf("браузер запущен: порт %d, pid %d", info.Port, info.PID))
		},
	}
	start.Flags().BoolVar(&opts.Headless, "headless", false, "без окна")
	start.Flags().StringVar(&opts.Profile, "profile", "", "каталог профиля (по умолчанию — рядом со state)")

	stop := &cobra.Command{
		Use:   "stop",
		Short: "Закрыть Chrome и удалить state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), stopTimeout)
			defer cancel()
			if err := pages.Stop(ctx, env); err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]bool{"stopped": true}, "браузер остановлен")
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Запущен ли Chrome на --port",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), statusTimeout)
			defer cancel()
			info, err := pages.Status(ctx, env)
			if err != nil {
				return err
			}
			human := "браузер не запущен"
			if info.Running {
				human = fmt.Sprintf("браузер запущен: порт %d, pid %d, вкладок %d", info.Port, info.PID, info.Pages)
			}
			return printResult(cmd, flags, info, human)
		},
	}

	cmd.AddCommand(start, stop, status)
	return cmd
}
