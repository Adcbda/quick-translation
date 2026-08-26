//go:build !windows

package main

import "os/exec"

func configureHiddenProcess(*exec.Cmd) {}
