package main

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/khalidhaykay/cmdforge"
)

func TestExampleRegistersAndRunsSeedCommand(t *testing.T) {
	cli := cmdforge.New(nil, migrations)
	cli.Register("seed", func(ctx context.Context) {
		// Example seed logic is intentionally for tests.
	}, false)

	prevArgs := os.Args
	defer func() { os.Args = prevArgs }()
	os.Args = []string{"cmd", "seed"}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() {
		_ = w.Close()
		os.Stderr = oldStderr
	}()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()

	cli.Start(context.Background())

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	<-done
}
