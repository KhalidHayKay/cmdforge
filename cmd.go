package cmdforge

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// CMD is the command router. It holds a map of named commands and dispatches
// to the right one based on os.Args.
type CMD struct {
	commands map[string]Command
}

func newCMD() *CMD {
	commands := make(map[string]Command)

	// "list" is always registered automatically so the caller can discover
	// what commands are available without reading source code.
	commands["list"] = Command{
		Run: func(ctx context.Context) {
			printCommands(commands)
		},
		Destructive: false,
	}

	return &CMD{
		commands: commands,
	}
}

// add registers a new command by name through CLI.Register.
func (c *CMD) add(name string, handler func(context.Context), destructive bool) {
	c.commands[name] = Command{
		Run:         handler,
		Destructive: destructive,
	}
}

// run reads os.Args, finds the matching command, handles destructive confirmation,
// and executes. This is intentionally kept simple — no flag parsing, no subcommands.
func (c *CMD) run(ctx context.Context) {
	input := strings.Join(os.Args[1:], " ")

	cmd, ok := c.commands[input]
	if !ok {
		log.Fatalf("Unknown command: %s", input)
	}

	if cmd.Destructive {
		fmt.Print("⚠️  This is a destructive operation. Type 'yes' to proceed: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')

		if strings.TrimSpace(response) != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	cmd.Run(ctx)
}

func printCommands(commands map[string]Command) {
	fmt.Println("Available commands:")

	excluded := map[string]bool{
		"list": true,
	}

	keys := make([]string, 0, len(commands))
	for k := range commands {
		if !excluded[k] {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	for _, cmd := range keys {
		fmt.Printf("- %s\n", cmd)
	}
}
