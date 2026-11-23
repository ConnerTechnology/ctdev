//go:build !unix

package backup

import "os/exec"

// resticProcess leaves exec's defaults: there is no interrupt to send a
// process, so cancelling kills it.
func resticProcess(cmd *exec.Cmd) {}
