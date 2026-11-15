package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gverdugo-dev/ine/internal/selfupdate"
)

func cmdUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	check := fs.Bool("check", false, "only say whether a newer release exists")
	want := fs.String("version", "", "install this release tag instead of the latest, for example v0.2.0")
	if err := fs.Parse(args); err != nil {
		return err
	}
	current := buildVersion()
	u := selfupdate.New()

	tag := *want
	if tag == "" {
		latest, err := u.Latest(ctx)
		if err != nil {
			return fmt.Errorf("update: %w", err)
		}
		tag = latest
		if !selfupdate.Newer(tag, current) {
			fmt.Printf("ine %s is up to date\n", current)
			return nil
		}
	}
	if *check {
		fmt.Printf("ine %s is available (installed: %s). Run: ine update\n", tag, current)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Downloading ine %s for this system...\n", tag)
	bin, err := u.Download(ctx, tag)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if err := selfupdate.Replace(exe, bin); err != nil {
		if errors.Is(err, selfupdate.ErrNotWritable) {
			return fmt.Errorf("update: %w; run it with the permissions of %s (for example sudo ine update)", err, filepath.Dir(exe))
		}
		return fmt.Errorf("update: %w", err)
	}
	fmt.Printf("Updated ine %s -> %s (%s)\n", current, tag, exe)
	return nil
}
