package main

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/spf13/cobra"
)

type logFlags struct {
	level, format string
}

func (f *logFlags) register(root *cobra.Command) {
	root.PersistentFlags().StringVar(&f.level, "log-level", "info", "Log level: debug, info, warn or error")
	root.PersistentFlags().StringVar(&f.format, "log-format", "text", "Log format on stderr: text or json")
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error { return f.install(cmd.ErrOrStderr()) }
}

func (f *logFlags) install(stderr io.Writer) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(f.level)); err != nil {
		return fmt.Errorf("--log-level %q: %w", f.level, err)
	}
	handler, err := logHandler(f.format, stderr, &slog.HandlerOptions{Level: level})
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(handler))
	return nil
}

func logHandler(format string, stderr io.Writer, options *slog.HandlerOptions) (slog.Handler, error) {
	switch format {
	case "text":
		return slog.NewTextHandler(stderr, options), nil
	case "json":
		return slog.NewJSONHandler(stderr, options), nil
	}
	return nil, fmt.Errorf("--log-format %q: want text or json", format)
}
