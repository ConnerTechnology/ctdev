//go:build !unix

package sysutil

import "os/exec"

func configureProcessGroup(*exec.Cmd) {}
