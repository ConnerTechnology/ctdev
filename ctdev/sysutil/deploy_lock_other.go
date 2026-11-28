//go:build !unix

package sysutil

import "sync"

var deployMu sync.Mutex

// Only Unix builds expose install and update. Serialize any direct library
// calls on other systems within the process.
func lockDeployDir(string, bool) (func(), error) {
	deployMu.Lock()
	return deployMu.Unlock, nil
}
