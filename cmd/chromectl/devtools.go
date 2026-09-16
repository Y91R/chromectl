package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/inspect"
	"github.com/Y91R/chromectl/internal/console"
)

func newConsoleCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Сообщения консоли выбранной вкладки",
	}

	var opts console.ListOptions
	list := &cobra.Command{
		Use:   "list",
		Short: "Список сообщений, накопленных страницей, включая залогированные до вызова",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			text, err := inspect.ConsoleList(ctx, env, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"messages": text}, text)
		},
	}
	list.Flags().StringSliceVar(&opts.Types, "types", nil, "только эти типы: log, error, warn, info, debug…")
	list.Flags().IntVar(&opts.PageSize, "page-size", 0, "сообщений на странице; 0 — все")
	list.Flags().IntVar(&opts.PageIdx, "page-idx", 0, "номер страницы с нуля")

	get := &cobra.Command{
		Use:   "get <msgid>",
		Short: "Сообщение целиком: аргументы и стек",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("msgid — целое число, получено %q", args[0])
			}
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			msg, err := inspect.ConsoleGet(ctx, env, id)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, msg, console.Detail(msg))
		},
	}

	cmd.AddCommand(list, get)
	return cmd
}

func newNetworkCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Сетевые запросы выбранной вкладки",
	}

	var listOpts inspect.NetworkListOptions
	list := &cobra.Command{
		Use:   "list",
		Short: "Запросы последнего --capture, без него — из Performance API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			text, err := inspect.NetworkList(ctx, env, listOpts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"requests": text}, text)
		},
	}
	list.Flags().StringSliceVar(&listOpts.Types, "types", nil, "только эти типы: document, fetch, xhr, script, image…")
	list.Flags().IntVar(&listOpts.PageSize, "page-size", 0, "запросов на странице; 0 — все")
	list.Flags().IntVar(&listOpts.PageIdx, "page-idx", 0, "номер страницы с нуля")

	var getOpts inspect.NetworkGetOptions
	get := &cobra.Command{
		Use:   "get <reqid>",
		Short: "Запрос из последнего --capture: заголовки, тела, ошибка",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("reqid — целое число, получено %q", args[0])
			}
			getOpts.ReqID = id
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			req, text, err := inspect.NetworkGet(ctx, env, getOpts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, req, text)
		},
	}
	get.Flags().StringVar(&getOpts.RequestFile, "request-file", "", "сохранить тело запроса в файл")
	get.Flags().StringVar(&getOpts.ResponseFile, "response-file", "", "сохранить тело ответа в файл")

	cmd.AddCommand(list, get)
	return cmd
}
