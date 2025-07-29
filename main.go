package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	cli "github.com/urfave/cli/v3"
	"golang.org/x/term"
)

func cmdConstructor(env string, flags []int64, yaml string, sessionID string) string {
	ram := flags[0]
	cpu := flags[1]

	// Create socket path for this session
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("dbox-%s.sock", sessionID))

	cmd := fmt.Sprintf("sudo qemu-system-x86_64 -enable-kvm -cpu host -m %dM -smp %d -nographic -kernel images/%s/%s.bzImage -initrd images/%s/%s.initrd -append \"console=ttyS0 root=/dev/vda rw\" -drive file=images/%s/%s.img,format=raw,if=virtio -fsdev local,id=fsdev0,path=$PWD,security_model=none -device virtio-9p-pci,fsdev=fsdev0,mount_tag=hostshare -machine q35,accel=kvm -monitor unix:%s,server,nowait", ram, cpu, env, env, env, env, env, env, socketPath)

	return cmd
}

// -----------------------------------------------------------------------------

func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

// -----------------------------------------------------------------------------

func findRunningSessions() ([]string, error) {
	socketsPattern := filepath.Join(os.TempDir(), "dbox-*.sock")
	matches, err := filepath.Glob(socketsPattern)
	if err != nil {
		return nil, err
	}

	var activeSessions []string
	for _, socketPath := range matches {
		// Check if socket is still active
		conn, err := net.Dial("unix", socketPath)
		if err == nil { //if there is NO error
			conn.Close()
			// Grab sesh ID
			basename := filepath.Base(socketPath)
			sessionID := strings.TrimPrefix(strings.TrimSuffix(basename, ".sock"), "dbox-")
			activeSessions = append(activeSessions, sessionID)
		} else {
			// Clean up dead sock
			os.Remove(socketPath)
		}
	}

	return activeSessions, nil
}

// -----------------------------------------------------------------------------

// Shared helper for handling detach sequence and terminal restore
func handleDetachAndIO(conn io.ReadWriteCloser, oldState *term.State) {
	detachChan := make(chan bool, 1)

	go func() {
		buf := make([]byte, 1024)
		detachSeq := []byte("<DBOX:DETACH>")
		window := make([]byte, 0, len(detachSeq))
		for {
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("error reading from vm: %v", err)
				}
				return
			}
			if n > 0 {
				os.Stdout.Write(buf[:n])
				if len(window)+n <= len(detachSeq) {
					window = append(window, buf[:n]...)
				} else {
					needed := len(detachSeq) - len(window)
					if needed > 0 {
						window = append(window, buf[:needed]...)
					}
					window = append(window, buf[needed:n]...)
					if len(window) > len(detachSeq) {
						window = window[len(window)-len(detachSeq):]
					}
				}
				if len(window) == len(detachSeq) && string(window) == string(detachSeq) {
					if oldState != nil {
						term.Restore(int(os.Stdin.Fd()), oldState)
					}
					fmt.Println("Detached from session")
					detachChan <- true
					return
				}
			}
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("error reading stdin: %v", err)
				}
				return
			}
			_, err = conn.Write(buf[:n])
			if err != nil {
				log.Printf("err writing to vm: %v", err)
				return
			}
		}
	}()

	<-detachChan
}

// -----------------------------------------------------------------------------

// Helper to combine io.ReadCloser and io.Writer into an io.ReadWriteCloser
// Used for startEnv to pass pipes to handleDetachAndIO
// Only Close() the reader (stdoutPipe) to avoid closing os.Stdin

type readWritePipe struct {
	reader io.ReadCloser
	writer io.Writer
}

func (rwp *readWritePipe) Read(p []byte) (int, error) {
	return rwp.reader.Read(p)
}
func (rwp *readWritePipe) Write(p []byte) (int, error) {
	return rwp.writer.Write(p)
}
func (rwp *readWritePipe) Close() error {
	return rwp.reader.Close()
}

// -----------------------------------------------------------------------------

func attachToSession(sessionID string) error {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("dbox-%s.sock", sessionID))

	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return fmt.Errorf("session %s not found", sessionID)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to session %s: %v", sessionID, err)
	}
	defer conn.Close()

	fmt.Printf("Attaching to session %s...\n", sessionID)

	// Use raw terminal mode (this gives us immediate keystroke access)
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("warning: couldn't set raw mode: %v", err)
	}
	defer func() {
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}()

	// Handle ctrl+c
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
		}
		os.Exit(0)
	}()

	//Connect back to qemu monitor and console
	conn, err = net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to connect to session: %v", err) // I'm not sure if this is gonna be a value
	}
	defer conn.Close()

	// Switch to console mode
	_, err = conn.Write([]byte("info status\n"))
	if err != nil {
		return fmt.Errorf("failed to query vm status: %v", err) // Same here idk type
	}

	time.Sleep(100 * time.Millisecond) // Avoid race conditions by making sure qemu's I/O is set up

	handleDetachAndIO(conn, oldState)
	return nil
}

// -----------------------------------------------------------------------------

func startEnv(env string, flags []int64, yaml string) error {
	// Generate unique session ID for this VM
	sessionID := generateSessionID()

	// Execute the command to start the environment
	cmdStr := cmdConstructor(env, flags, yaml, sessionID)
	cmd := exec.Command("bash", "-c", cmdStr)

	// Connect stdout to the current process
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %v", err)
	}

	// Get a pipe for writing to the process's stdin
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %v", err)
	}

	cmd.Stdin = os.Stdin // For normal input (e.g. signals)

	// Set up raw terminal mode for proper handling of escape sequences
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		log.Printf("Warning: could not set raw mode: %v", err)
	}
	defer func() {
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
		}
	}()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
		}
		os.Exit(0)
	}()

	// Run the command and wait for it to complete
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start environment: %v", err)
	}

	detachChan := make(chan bool, 1)

	// Use raw byte copying instead of line-based scanning for real-time output
	go func() {
		rwp := &readWritePipe{reader: stdoutPipe, writer: stdinPipe}
		handleDetachAndIO(rwp, oldState)
		detachChan <- true
	}()

	// Wait for detach or process exit
	go func() {
		cmd.Wait()
		detachChan <- false
	}()

	detached := <-detachChan
	stdinPipe.Close()
	if detached {
		fmt.Println("Detached from devbox. Use 'ps aux | grep qemu' to check if it's still running.")
		return nil
	}

	fmt.Println("Devbox shut down successfully.")
	return nil
}

// -----------------------------------------------------------------------------

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
