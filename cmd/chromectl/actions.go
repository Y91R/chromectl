package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/input"
	"github.com/Y91R/chromectl/internal/command/pages"
	"github.com/Y91R/chromectl/internal/command/script"
	"github.com/Y91R/chromectl/internal/command/session"
)

const (
	actionTimeout = 30 * time.Second
	evalTimeout   = 60 * time.Second
)

type actionFlags struct {
	dialog     string
	snapshot   bool
	capture    bool
	unredacted bool
}

func (a *actionFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&a.dialog, "dialog", "accept", "ответ на диалог: accept, dismiss или текст для prompt")
	cmd.Flags().BoolVar(&a.snapshot, "snapshot", false, "после действия напечатать свежий снапшот")
	cmd.Flags().BoolVar(&a.capture, "capture", false, "записать сетевые запросы действия для network list/get")
	cmd.Flags().BoolVar(&a.unredacted, "unredacted", false, "с --capture: не маскировать Authorization и cookie")
}

func (a *actionFlags) common(cmd *cobra.Command) input.Common {
	return input.Common{
		Dialog:     session.DialogFlag{Value: a.dialog, Set: cmd.Flags().Changed("dialog")},
		Snapshot:   a.snapshot,
		Capture:    a.capture,
		Unredacted: a.unredacted,
	}
}

type actionFunc func(ctx context.Context, env pages.Env, common input.Common) (input.Result, error)

func runAction(cmd *cobra.Command, flags *globalFlags, af *actionFlags, done string, action actionFunc) error {
	env, err := flags.env()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
	defer cancel()
	res, err := action(ctx, env, af.common(cmd))
	if err != nil {
		return err
	}
	human := dialogLines(res.Dialogs) + done
	if res.Snapshot != "" {
		human += "\n" + strings.TrimRight(res.Snapshot, "\n")
	}
	return printResult(cmd, flags, res, human)
}

func newClickCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	var double bool
	cmd := &cobra.Command{
		Use:   "click <uid>",
		Short: "Кликнуть по элементу из снапшота",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, "клик по "+args[0], func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Click(ctx, env, c, args[0], double)
			})
		},
	}
	cmd.Flags().BoolVar(&double, "dbl", false, "двойной клик")
	af.bind(cmd)
	return cmd
}

func newHoverCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	cmd := &cobra.Command{
		Use:   "hover <uid>",
		Short: "Навести курсор на элемент",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, "курсор над "+args[0], func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Hover(ctx, env, c, args[0])
			})
		},
	}
	af.bind(cmd)
	return cmd
}

func newDragCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	cmd := &cobra.Command{
		Use:   "drag <from-uid> <to-uid>",
		Short: "Перетащить элемент на другой",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, fmt.Sprintf("перетащено %s → %s", args[0], args[1]), func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Drag(ctx, env, c, args[0], args[1])
			})
		},
	}
	af.bind(cmd)
	return cmd
}

func newFillCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	cmd := &cobra.Command{
		Use:   "fill <uid> <value>",
		Short: "Заполнить поле, выбрать вариант select или отметить чекбокс (true/false)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, "заполнено "+args[0], func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Fill(ctx, env, c, args[0], args[1])
			})
		},
	}
	af.bind(cmd)
	return cmd
}

func newFillFormCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	cmd := &cobra.Command{
		Use:   "fill-form <uid=value>...",
		Short: "Заполнить несколько элементов формы одной командой",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, fmt.Sprintf("заполнено элементов: %d", len(args)), func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.FillForm(ctx, env, c, args)
			})
		},
	}
	af.bind(cmd)
	return cmd
}

func newTypeCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	var submit string
	cmd := &cobra.Command{
		Use:   "type <text>",
		Short: "Напечатать текст в элемент с фокусом",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, "напечатано", func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Type(ctx, env, c, args[0], submit)
			})
		},
	}
	cmd.Flags().StringVar(&submit, "submit", "", "клавиша после текста, например Enter")
	af.bind(cmd)
	return cmd
}

func newPressCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	cmd := &cobra.Command{
		Use:   "press <key>",
		Short: "Нажать клавишу или сочетание: Enter, Control+A, Control++",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, "нажато "+args[0], func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Press(ctx, env, c, args[0])
			})
		},
	}
	af.bind(cmd)
	return cmd
}

func newUploadCmd(flags *globalFlags) *cobra.Command {
	var af actionFlags
	cmd := &cobra.Command{
		Use:   "upload <uid> <file>...",
		Short: "Загрузить файлы через input[type=file] или элемент, открывающий выбор файла",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAction(cmd, flags, &af, "загружено: "+strings.Join(args[1:], ", "), func(ctx context.Context, env pages.Env, c input.Common) (input.Result, error) {
				return input.Upload(ctx, env, c, args[0], args[1:])
			})
		},
	}
	af.bind(cmd)
	return cmd
}

func newSnapshotCmd(flags *globalFlags) *cobra.Command {
	var verbose bool
	var output string
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Текстовый снапшот выбранной вкладки по дереву доступности, с uid",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), actionTimeout)
			defer cancel()
			res, err := script.Snapshot(ctx, env, verbose, output)
			if err != nil {
				return err
			}
			human := strings.TrimRight(res.Snapshot, "\n")
			if res.Path != "" {
				human = res.Path
			}
			return printResult(cmd, flags, res, human)
		},
	}
	cmd.Flags().BoolVar(&verbose, "verbose", false, "все узлы дерева доступности, а не только значимые")
	cmd.Flags().StringVarP(&output, "output", "o", "", "записать снапшот в файл")
	return cmd
}

func newEvalCmd(flags *globalFlags) *cobra.Command {
	var opts script.EvalOptions
	var dialog string
	cmd := &cobra.Command{
		Use:   "eval <function>",
		Short: "Выполнить JS-функцию на странице; результат — JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			opts.Function = args[0]
			opts.Dialog = session.DialogFlag{Value: dialog, Set: cmd.Flags().Changed("dialog")}
			ctx, cancel := context.WithTimeout(cmd.Context(), evalTimeout)
			defer cancel()
			res, err := script.Eval(ctx, env, opts)
			if err != nil {
				return err
			}
			human := res.Value
			if res.Path != "" {
				human = res.Path
			}
			// stdout eval — только JSON-значение, чтобы его можно было разобрать;
			// закрытые диалоги — в stderr.
			if !flags.json {
				if _, err := fmt.Fprint(cmd.ErrOrStderr(), dialogLines(res.Dialogs)); err != nil {
					return err
				}
			}
			return printResult(cmd, flags, res, human)
		},
	}
	cmd.Flags().StringArrayVar(&opts.Args, "arg", nil, "uid элемента, передаваемого функции аргументом (можно несколько)")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", "", "записать результат в файл")
	cmd.Flags().StringVar(&dialog, "dialog", "accept", "ответ на диалог: accept, dismiss или текст для prompt")
	return cmd
}

func newDialogCmd(flags *globalFlags) *cobra.Command {
	var text string
	cmd := &cobra.Command{
		Use:   "dialog accept|dismiss",
		Short: "Ответ на диалог для следующей команды (одноразово)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "accept" && args[0] != "dismiss" {
				return errors.New("ожидалось accept или dismiss")
			}
			env, err := flags.env()
			if err != nil {
				return err
			}
			if err := script.SetDialog(env, args[0] == "accept", text); err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"next": args[0]}, "следующий диалог: "+args[0])
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "текст для prompt (только с accept)")
	return cmd
}

func newWaitForCmd(flags *globalFlags) *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "wait-for <text>...",
		Short: "Дождаться, пока на странице появится любой из текстов",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := flags.env()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout+actionTimeout)
			defer cancel()
			found, err := script.WaitFor(ctx, env, args, timeout)
			if err != nil {
				return err
			}
			return printResult(cmd, flags, map[string]string{"found": found}, "найден текст: "+found)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "сколько ждать")
	return cmd
}
