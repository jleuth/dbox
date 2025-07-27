package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	cli "github.com/urfave/cli/v3"
)

func startEnv(env string) string {
	qemuCmd := exec.Command("sh", "-c", fmt.Sprintf("sleep 3", env))
	if err := qemuCmd.Run(); err != nil {
		log.Printf("Error running command: %v", err)
		return "error"
	}
	return qemuCmd.String()
}

func main() {
	cmd := &cli.Command{
		Name:  "dbox",
		Usage: "Ephemeral development environment manager",
		Commands: []*cli.Command{
			{
				Name:    "run",
				Aliases: []string{"r"},
				Usage:   "run <environment> - spin up a new devbox with specified environment",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args()
					if args.Len() == 0 {
						return fmt.Errorf("please specify an environment: go, rust, node, python, etc")
					}

					env := args.Get(0)
					fmt.Printf("Spinning up devbox with %s environment...\n", env)
					fmt.Printf(startEnv(env))
					return nil
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
