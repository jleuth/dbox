package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	cli "github.com/urfave/cli/v3"
)

func cmdConstructor(env string, flags []int64, yaml string) string {
	ram := flags[0]
	cpu := flags[1]

	cmd := fmt.Sprintf("sudo qemu-system-x86_64 -enable-kvm -cpu host -m %dM -smp %d -nographic -kernel images/%s/%s.bzImage -initrd images/%s/%s.initrd -append \"console=ttyS0 root=/dev/vda rw\" -drive file=images/%s/%s.img,format=raw,if=virtio -fsdev local,id=fsdev0,path=$PWD,security_model=none -device virtio-9p-pci,fsdev=fsdev0,mount_tag=hostshare -machine q35,accel=kvm", ram, cpu, env, env, env, env, env, env)

	return cmd
}

func startEnv(env string, flags []int64, yaml string) error {
	// Execute the command to start the environment
	cmdStr := cmdConstructor(env, flags, yaml)
	cmd := exec.Command("bash", "-c", cmdStr)

	// Connect stdout and stderr to the current process
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Run the command and wait for it to complete
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start environment: %v", err)
	}

	return nil
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
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:     "ram",
						Usage:    "RAM size in MB, default value: 2048",
						Value:    2048,
						Required: false,
					},
					&cli.IntFlag{
						Name:     "cpu",
						Usage:    "CPU cores (vCPU), default value: 2",
						Value:    2,
						Required: false,
					},
					&cli.StringFlag{
						Name:     "yaml",
						Usage:    "Path to YAML file for a custom environment config",
						Required: false,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args()
					if args.Len() == 0 {
						return fmt.Errorf("please specify an environment: go, rust, node, python, etc")
					}

					env := args.Get(0)
					ram := cmd.Int("ram")
					cpu := cmd.Int("cpu")

					fmt.Printf("Spinning up devbox with %s environment (RAM: %d MB, CPU: %d vCPU)...\n", env, ram, cpu)
					startEnv(env, []int64{int64(ram), int64(cpu)}, cmd.String("yaml"))
					return nil
				},
			},
			// {
			// 	Name:    "fetch",
			// 	Aliases: []string{"f"},
			// 	Usage:   "fetch <environment> <save_directory>- fetch the latest devbox image for the specified environment",
			// 	Flags: []cli.Flag{
			// 		&cli.StringFlag{
			// 			Name:     "env",
			// 			Usage:    "Environment to fetch (e.g., go, rust, node, python)",
			// 			Required: true,
			// 		},
			// 		&cli.StringFlag{
			// 			Name:     "save_directory",
			// 			Usage:    "Directory to save the fetched image",
			// 			Required: true,
			// 		},
			// 	},
			// 	Action: func(ctx context.Context, c *cli.Command) error {
			// 		env := c.String("env")
			// 		saveDir := c.String("save_directory")

			// 		fmt.Printf("Fetching %s environment to %s\n", env, saveDir)
			// 		return nil
			// 	},
			// },
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
