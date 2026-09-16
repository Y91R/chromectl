package main

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/diagnose"
	"github.com/Y91R/chromectl/internal/perf"
)

const (
	traceExtraTimeout = 90 * time.Second
	heapTimeout       = 2 * time.Minute
	lighthouseTimeout = 5 * time.Minute
)

func newPerfCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "perf",
		Short: "Трейс производительности и его разборы",
	}

	var opts diagnose.TraceOptions
	trace := &cobra.Command{
		Use:   "trace",
		Short: "Записать трейс выбранной вкладки и посчитать LCP, FCP, TTFB, CLS, длинные задачи",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), opts.Duration+traceExtraTimeout)
			defer cancel()
			res, err := diagnose.Trace(ctx, env, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, res, perf.Summary(res.Metrics)+"\nФайл трейса: "+res.Path)
		},
	}
	trace.Flags().BoolVar(&opts.Reload, "reload", true, "перезагрузить страницу после начала записи, чтобы в трейс попала загрузка")
	trace.Flags().DurationVar(&opts.Duration, "duration", 5*time.Second, "сколько писать после загрузки")
	trace.Flags().StringVarP(&opts.Output, "output", "o", "", "файл трейса (.json или .json.gz); без него — временный файл")

	insight := &cobra.Command{
		Use:   "insight <trace-file> <LCPBreakdown|LayoutShifts|LongTasks>",
		Short: "Разбор сохранённого трейса; браузер не нужен",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			text, err := diagnose.Insight(args[0], args[1])
			if err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"insight": args[1], "text": text}, text)
		},
	}

	cmd.AddCommand(trace, insight)
	return cmd
}

func newHeapCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "heap",
		Short: "Память страницы",
	}

	var output string
	snapshot := &cobra.Command{
		Use:   "snapshot",
		Short: "Сохранить heap snapshot выбранной вкладки (открывается во вкладке Memory DevTools)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), heapTimeout)
			defer cancel()
			path, err := diagnose.HeapSnapshot(ctx, env, output)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"path": path}, "heap snapshot сохранён в "+path)
		},
	}
	snapshot.Flags().StringVarP(&output, "output", "o", "", "файл .heapsnapshot")
	_ = snapshot.MarkFlagRequired("output")

	cmd.AddCommand(snapshot)
	return cmd
}

func newAuditCmd(flags *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Аудиты страницы",
	}

	opts := diagnose.LighthouseOptions{}
	lighthouse := &cobra.Command{
		Use:   "lighthouse",
		Short: "Lighthouse: доступность, SEO, best practices, agentic browsing (через npx)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), lighthouseTimeout)
			defer cancel()
			res, err := diagnose.Lighthouse(ctx, env, opts)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, res, res.Text())
		},
	}
	lighthouse.Flags().StringVar(&opts.Device, "device", "desktop", "desktop или mobile")
	lighthouse.Flags().StringVar(&opts.Mode, "mode", "navigation", "navigation (перезагрузка и аудит); snapshot CLI Lighthouse не поддерживает")
	lighthouse.Flags().StringVar(&opts.OutputDir, "output-dir", "", "каталог для report.json и report.html; без него — временный")

	cmd.AddCommand(lighthouse)
	return cmd
}
