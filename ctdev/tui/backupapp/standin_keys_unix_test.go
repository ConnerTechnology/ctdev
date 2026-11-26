//go:build unix

package backupapp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// keyStandin is a restic stand-in that keeps a repository's keys, so the
// account actions run for real against it: each key is a file under
// <root>/<repository>/keys holding its password, user, host and creation
// time. A key opens the repository when RESTIC_PASSWORD is its password;
// otherwise restic exits 12, as for a wrong password. A file named fail-<cmd>
// in the repository makes `restic key <cmd>` fail, and a B2 repository
// opens only when B2_ACCOUNT_KEY isn't "wrong". snapshots prints the
// repository's snapshots.json, and each call is logged to <root>/restic.log.
const keyStandinScript = `#!/bin/sh
root="${STANDIN_ROOT:-$HOME}"
echo "$*" >> "$root/restic.log"
repo="$root/$(printf '%s' "$RESTIC_REPOSITORY" | tr '/:' '__')"
auth() {
  if [ "$B2_ACCOUNT_KEY" = wrong ]; then echo "Fatal: unable to open B2 bucket: unauthorized" >&2; exit 1; fi
  for f in "$repo"/keys/*; do
    [ -f "$f" ] || continue
    if [ "$(sed -n 1p "$f")" = "$RESTIC_PASSWORD" ]; then cur=$(basename "$f"); return 0; fi
  done
  echo "Fatal: wrong password or no key found" >&2; exit 12
}
addkey() {
  id=$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')
  printf '%s\n%s\n%s\n%s\n' "$1" "$2" "$3" "$(date '+%Y-%m-%d %H:%M:%S')" > "$repo/keys/$id"
  echo "$id"
}
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
init)
  if [ -d "$repo/keys" ]; then echo "Fatal: config file already exists" >&2; exit 1; fi
  mkdir -p "$repo/keys"; addkey "$RESTIC_PASSWORD" init init > /dev/null
  echo '{"message_type":"initialized","id":"0123456789abcdef","repository":"example"}' ;;
key)
  if [ "$2" = list ] && [ ! -d "$repo/keys" ]; then echo '[]'; exit 0; fi
  auth
  if [ -e "$repo/fail-$2" ]; then echo "Fatal: example failure" >&2; exit 1; fi
  case "$2" in
  list)
    sep=''; printf '['
    for f in "$repo"/keys/*; do
      [ -f "$f" ] || continue
      id=$(basename "$f"); c=false; [ "$id" = "$cur" ] && c=true
      printf '%s{"id":"%s","current":%s,"userName":"%s","hostName":"%s","created":"%s"}' "$sep" "$id" "$c" "$(sed -n 2p "$f")" "$(sed -n 3p "$f")" "$(sed -n 4p "$f")"
      sep=,
    done
    echo ']' ;;
  add)
    shift 2; user=; host=
    while [ $# -gt 0 ]; do case "$1" in --user) user=$2; shift ;; --host) host=$2; shift ;; esac; shift; done
    pw=$(cat <&3)
    echo "saved new key with ID $(addkey "$pw" "$user" "$host")" ;;
  remove)
    for id; do :; done
    if [ "$id" = "$cur" ]; then echo "Fatal: refusing to remove the key currently used" >&2; exit 1; fi
    rm "$repo/keys/$id" ;;
  esac ;;
snapshots)
  if [ -d "$repo/keys" ]; then auth; fi
  if [ -f "$repo/snapshots.json" ]; then
    cat "$repo/snapshots.json"
  elif [ -f "$HOME/snapshots.json" ]; then
    case "$*" in *--tag*) echo '[]' ;; *) cat "$HOME/snapshots.json" ;; esac
  else
    echo '[]'
  fi ;;
stats)
  if [ -d "$repo/keys" ]; then auth; fi
  echo '{"total_size": 2048}' ;;
esac
`

// removeStandinHostKey removes any key belonging to hostname from address's stand-in repository.
func removeStandinHostKey(t *testing.T, root, address, hostname string) {
	t.Helper()
	keys, _ := filepath.Glob(filepath.Join(standinRepo(root, address), "keys", "*"))
	for _, k := range keys {
		data, err := os.ReadFile(k)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		if len(lines) >= 3 && strings.TrimSpace(lines[2]) == hostname {
			_ = os.Remove(k)
		}
	}
}

// keyStandin writes the stand-in and returns where it keeps repositories.
func keyStandin(t *testing.T) (bin, root string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("STANDIN_ROOT", root)
	bin = filepath.Join(root, "restic")
	if err := os.WriteFile(bin, []byte(keyStandinScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, root
}

// standinRepo is where the stand-in keeps address's repository.
func standinRepo(root, address string) string {
	return filepath.Join(root, strings.NewReplacer("/", "_", ":", "_").Replace(address))
}

// machineOn is a computer named host in its own temporary home, running the
// stand-in.
func machineOn(t *testing.T, bin, host string) *backup.Machine {
	t.Helper()
	home := t.TempDir()
	return &backup.Machine{Dir: filepath.Join(home, ".config", "ctdev", "backup"), Home: home,
		Hostname: host, Username: "example", Restic: bin, Opts: sysutil.Opts{Stdout: &bytes.Buffer{}}}
}

const exampleMaster = "correct horse example staple"

// accountFixture is example-backups at address, created on example-laptop
// and connected from example-desktop and example-server.
type accountFixture struct {
	root                    string
	laptop, desktop, server *backup.Machine
	dest                    *backup.Destination // the laptop's
	kit                     string              // as created
}

func newAccountFixture(t *testing.T, address string, creds map[string]string) accountFixture {
	t.Helper()
	bin, root := keyStandin(t)
	ctx := context.Background()
	f := accountFixture{root: root, laptop: machineOn(t, bin, "example-laptop"),
		desktop: machineOn(t, bin, "example-desktop"), server: machineOn(t, bin, "example-server")}
	setup, err := f.laptop.CreateDestination(ctx, backup.NewDestination{Name: "example-backups", Address: address, Credentials: creds}, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	f.dest, f.kit = setup.Destination, setup.Kit
	for _, m := range []*backup.Machine{f.desktop, f.server} {
		if _, err := m.ConnectDestination(ctx, setup.Kit, exampleMaster); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// computer is host as the laptop's Computers lists it.
func (f accountFixture) computer(t *testing.T, host string) backup.Computer {
	t.Helper()
	cs, err := f.dest.Computers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Hostname == host {
			return c
		}
	}
	t.Fatalf("no %s in %+v", host, cs)
	return backup.Computer{}
}

func secretKeyOf(t *testing.T, kit string) string {
	t.Helper()
	k, err := backup.ParseKit(kit)
	if err != nil {
		t.Fatal(err)
	}
	return k.SecretKey
}
