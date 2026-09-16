package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/pages"
)

const screenshotTimeout = 30 * time.Second

func newScreenshotCmd(flags *globalFlags) *cobra.Command {
	var opts pages.ScreenshotOptions
	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Скриншот выбранной вкладки; без -o — во временный файл",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), screenshotTimeout)
			defer cancel()
			res, err := pages.Screenshot(ctx, env, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, res, res.Path)
		},
	}
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "файл для сохранения")
	cmd.Flags().BoolVar(&opts.FullPage, "full-page", false, "вся страница, а не только видимая область")
	cmd.Flags().StringVar(&opts.Format, "format", "png", "png, jpeg или webp")
	cmd.Flags().IntVar(&opts.Quality, "quality", 0, "качество 0–100 для jpeg и webp")
	cmd.Flags().StringVar(&opts.UID, "uid", "", "снять только элемент из последнего снапшота")
	return cmd
}
