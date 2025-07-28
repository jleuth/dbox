# Devbox (dbox)

Ephemeral development environments that don't mess with your host system.

## What it does

Spins up QEMU VMs with pre-built Linux environments for different programming languages. Your current directory gets mounted inside the VM so you can work on your code without installing language runtimes, dependencies, or development tools on your actual machine.

## Usage

```bash
# Basic usage
dbox run node     # Start a Node.js environment
dbox run go       # Start a Go environment  
dbox run python   # Start a Python environment
dbox run rust     # Start a Rust environment

# With custom resources
dbox run node --ram 4096 --cpu 4

# Custom environment (coming soon)
dbox run custom --yaml my-env.yaml
```

## Why

Because dependency hell is real, language versions conflict, and sometimes you just want a clean slate without Docker's overhead or complexity. VMs are fast when you use KVM properly.

Your host filesystem is available at `/hostshare` inside the VM. No syncing, no copying, just direct access to your files.

## Requirements

- Linux host with KVM support
- QEMU installed
- Sufficient RAM (default: 2GB per VM, can be as low as 512MB)
- Pre-built environment images will be downloadable (coming soon)

Built for developers who value simplicity over features.