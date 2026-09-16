package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/emulate"
	"github.com/Y91R/chromectl/internal/emulation"
)

func newEmulateCmd(flags *globalFlags) *cobra.Command {
	var viewport, colorScheme, network, cpu, userAgent, geolocation, headers string
	var show bool
	cmd := &cobra.Command{
		Use:   "emulate",
		Short: "Эмуляция выбранной вкладки; меняются только переданные флаги",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			changed := func(name string, value *string) *string {
				if cmd.Flags().Changed(name) {
					return value
				}
				return nil
			}
			ch := emulation.Changes{
				Viewport:    changed("viewport", &viewport),
				ColorScheme: changed("color-scheme", &colorScheme),
				Network:     changed("network", &network),
				CPU:         changed("cpu", &cpu),
				UserAgent:   changed("user-agent", &userAgent),
				Geolocation: changed("geolocation", &geolocation),
				Headers:     changed("headers", &headers),
			}
			if !show && ch == (emulation.Changes{}) {
				return errors.New("укажите хотя бы один флаг эмуляции или --show")
			}
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			settings, err := emulate.Emulate(ctx, env, ch)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, settings, emulation.Describe(settings))
		},
	}
	cmd.Flags().StringVar(&viewport, "viewport", "", `<ширина>x<высота>[x<плотность>][,mobile][,touch][,landscape]; "" — сброс`)
	cmd.Flags().StringVar(&colorScheme, "color-scheme", "", "dark, light; auto — сброс")
	cmd.Flags().StringVar(&network, "network", "", "Offline, Slow 3G, Fast 3G, Slow 4G, Fast 4G; none — сброс")
	cmd.Flags().StringVar(&cpu, "cpu", "", "замедление CPU от 1 до 20; 1 — сброс")
	cmd.Flags().StringVar(&userAgent, "user-agent", "", `user agent; "" — сброс`)
	cmd.Flags().StringVar(&geolocation, "geolocation", "", `<широта>,<долгота>; "" — сброс`)
	cmd.Flags().StringVar(&headers, "headers", "", `дополнительные заголовки JSON-объектом; "" — сброс`)
	cmd.Flags().BoolVar(&show, "show", false, "показать текущую эмуляцию вкладки")
	return cmd
}

func newResizeCmd(flags *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "resize <width> <height>",
		Short: "Изменить размер страницы (окна вкладки)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			width, errW := strconv.Atoi(args[0])
			height, errH := strconv.Atoi(args[1])
			if errW != nil || errH != nil {
				return fmt.Errorf("ширина и высота — целые числа, получено %q и %q", args[0], args[1])
			}
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			if err := emulate.Resize(ctx, env, width, height); err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]int{"width": width, "height": height}, fmt.Sprintf("размер страницы %dx%d", width, height))
		},
	}
}
