package cmdforge

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	os.Stdout = w
	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = oldStdout
	return <-outC
}

func withStdin(input string, fn func()) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}

	_, _ = w.WriteString(input)
	_ = w.Close()
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	fn()
}

func TestNewCMDRegistersList(t *testing.T) {
	cmd := newCMD()
	if _, ok := cmd.commands["list"]; !ok {
		t.Fatal("expected list command to be registered")
	}

	output := captureStdout(t, func() {
		cmd.commands["list"].Run(context.Background())
	})

	if !strings.Contains(output, "Available commands:") {
		t.Fatalf("expected list output, got: %q", output)
	}
}

func TestCMDAddAndRun(t *testing.T) {
	cmd := newCMD()
	var executed bool
	cmd.add("hello", func(ctx context.Context) {
		executed = true
	}, false)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"prog", "hello"}

	cmd.run(context.Background())

	if !executed {
		t.Fatal("expected hello command to run")
	}
}

func TestCMDRunDestructiveConfirmation(t *testing.T) {
	cmd := newCMD()
	var executed bool
	cmd.add("delete", func(ctx context.Context) {
		executed = true
	}, true)

	oldArgs := os.Args
	oldStdin := os.Stdin
	defer func() {
		os.Args = oldArgs
		os.Stdin = oldStdin
	}()

	os.Args = []string{"prog", "delete"}

	output := captureStdout(t, func() {
		withStdin("no\n", func() {
			cmd.run(context.Background())
		})
	})

	if executed {
		t.Fatal("expected destructive command not to run after abort")
	}
	if !strings.Contains(output, "This is a destructive operation") || !strings.Contains(output, "Aborted.") {
		t.Fatalf("unexpected output for aborted destructive command: %q", output)
	}

	executed = false
	output = captureStdout(t, func() {
		withStdin("yes\n", func() {
			cmd.run(context.Background())
		})
	})

	if !executed {
		t.Fatal("expected destructive command to run after confirmation")
	}
	if !strings.Contains(output, "This is a destructive operation") {
		t.Fatalf("expected confirmation prompt, got: %q", output)
	}
}

func TestCLIRegisterAndStart(t *testing.T) {
	cli := New()
	var executed bool
	cli.Register("seed", func(ctx context.Context) {
		executed = true
	}, false)

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"prog", "seed"}

	cli.Start(context.Background())

	if !executed {
		t.Fatal("expected registered CLI command to run")
	}
}

// An application-owned group needs only Register; it has no lifecycle hooks.
type testCommands struct {
	name string
	run  func(context.Context)
}

func (g testCommands) Register(cli *CLI) {
	cli.Register(g.name, g.run, false)
}

var _ Extension = testCommands{}

func TestCLIComposition(t *testing.T) {
	cli := New()
	if len(cli.cmd.commands) != 1 {
		t.Fatalf("new CLI should only register list: %v", cli.cmd.commands)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls []string
	handler := func(name string) func(context.Context) {
		return func(got context.Context) {
			if got != ctx {
				t.Error("handler did not receive Start's context")
			}
			calls = append(calls, name)
		}
	}
	cli.Register("cache:clear", handler("cache"), false)
	testCommands{"user:create", handler("user")}.Register(cli)
	cli.Use(testCommands{"queue drain", handler("queue")})
	cli.Use(testCommands{"report generate", handler("report")})
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	for _, name := range []string{"cache:clear", "user:create", "queue drain", "report generate"} {
		os.Args = append([]string{"app"}, strings.Fields(name)...)
		cli.Start(ctx)
	}
	if got := strings.Join(calls, ","); got != "cache,user,queue,report" {
		t.Fatalf("calls = %s", got)
	}
	os.Args = []string{"app", "list"}
	output := captureStdout(t, func() { cli.Start(ctx) })
	if output != "Available commands:\n- cache:clear\n- queue drain\n- report generate\n- user:create\n" {
		t.Fatalf("list = %q", output)
	}
}

func TestRegisterReplacesCommand(t *testing.T) {
	cli := New()
	cli.Use(testCommands{"task", func(context.Context) { t.Fatal("old command ran") }})
	called := false
	cli.Register("task", func(context.Context) { called = true }, true)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"app", "task"}
	withStdin("no\n", func() { cli.Start(context.Background()) })
	if called {
		t.Fatal("replacement lost destructive flag")
	}
	withStdin("yes\n", func() { cli.Start(context.Background()) })
	if !called {
		t.Fatal("replacement did not run")
	}
}
