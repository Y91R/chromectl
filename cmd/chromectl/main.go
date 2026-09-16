// Команда chromectl управляет Chrome по CDP: каждый вызов подключается к
// debug-порту, выполняет одно действие и завершается.
//
// ADR: docs/adr/0009-cli-bez-demona.md — резидентный процесс один, сам Chrome.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Y91R/chromectl/internal/command/pages"
	"github.com/Y91R/chromectl/internal/state"
)

var version = "dev"

type globalFlags struct {
	port int
	page string
	json bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	root := newRootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	// Ошибка печатается один раз здесь: cobra сама не пишет ни ошибку, ни usage,
	// иначе stdout команды засорялся бы справкой.
	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, "chromectl:", err)
		return 1
	}
	return 0
}

func newRootCmd() *cobra.Command {
	flags := &globalFlags{}
	root := &cobra.Command{
		Use:           "chromectl",
		Short:         "Управление Chrome по CDP: одна команда — одно подключение",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().IntVar(&flags.port, "port", 9222, "debug-порт Chrome")
	root.PersistentFlags().StringVar(&flags.page, "page", "", "id вкладки вместо выбранной")
	root.PersistentFlags().BoolVar(&flags.json, "json", false, "вывод в JSON")
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddCommand(
		newVersionCmd(flags),
		newBrowserCmd(flags),
		newPagesCmd(flags),
		newNavigateCmd(flags),
		newScreenshotCmd(flags),
		newSnapshotCmd(flags),
		newClickCmd(flags),
		newHoverCmd(flags),
		newDragCmd(flags),
		newFillCmd(flags),
		newFillFormCmd(flags),
		newTypeCmd(flags),
		newPressCmd(flags),
		newUploadCmd(flags),
		newEvalCmd(flags),
		newDialogCmd(flags),
		newWaitForCmd(flags),
		newEmulateCmd(flags),
		newResizeCmd(flags),
		newConsoleCmd(flags),
		newNetworkCmd(flags),
		newPerfCmd(flags),
		newHeapCmd(flags),
		newAuditCmd(flags),
	)
	return root
}

func (g *globalFlags) env() (pages.Env, error) {
	base, err := stateBaseDir()
	if err != nil {
		return pages.Env{}, err
	}
	return pages.Env{Store: state.NewStore(base, g.port), Port: g.port, Page: g.page}, nil
}

// stateBaseDir — CHROMECTL_STATE_DIR или ~/.cache/chromectl. Переменная нужна
// e2e и параллельным агентам, чтобы не делить один state.
func stateBaseDir() (string, error) {
	if dir := os.Getenv("CHROMECTL_STATE_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("домашний каталог для state: %w", err)
	}
	return filepath.Join(home, ".cache", "chromectl"), nil
}
