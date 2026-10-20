// import-song adds an operator-owned default track to the shared playlist.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"musicmirror/server/internal/bootstrap"
	"musicmirror/server/internal/config"
)

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/import-song path/to/song.mp3")
		os.Exit(2)
	}
	if err := run(flag.Arg(0)); err != nil {
		slog.Error("import failed", "error", err)
		os.Exit(1)
	}
}
func run(filename string) error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	if c.DatabaseURL == "" {
		return fmt.Errorf("default song import requires Supabase configuration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	runtime, err := bootstrap.Open(ctx, c, slog.Default())
	if err != nil {
		return err
	}
	defer runtime.Close()
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	song, err := runtime.Catalog.Upload(ctx, "", filepath.Base(filename), f)
	if err != nil {
		return err
	}
	fmt.Printf("Imported %s (%s)\n", song.Name, song.ID)
	return nil
}
