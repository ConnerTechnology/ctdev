package backup

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
)

// The account actions of ADR-0016. restic gives every key full power, so
// the gate is ctdev's: each action is a method of Account, and the only way
// to get an Account is Unlock, which opens the repository with the account
// key derived from the master password. The exceptions don't need the
// master password: a Recovery Code reset (Recover, RecoverFromKit), and
// pasting an updated kit on a computer whose own key still opens the
// repository (UpdateFromKit).

// ErrThisComputer refuses removing the key this computer runs on: it would
// lock this computer out of the destination it's acting on.
var ErrThisComputer = errors.New("that's this computer's own key: remove it from another computer that's set up")

// Account is a destination unlocked with its master password.
type Account struct {
	d        *Destination
	master   string
	password string // the account key's restic password
	keyID    string // the account key's ID
}

// Unlock verifies the master password by opening the repository with the
// account key it and this computer's Secret Key make. A wrong password, or a
// Secret Key that has since been regenerated elsewhere, is ErrWrongPassword.
func (d *Destination) Unlock(ctx context.Context, master string) (*Account, error) {
	if d.m.Opts.DryRun {
		return nil, ErrDryRun
	}
	sk, err := parseSecretKey(d.secretKey)
	if err != nil {
		return nil, err
	}
	password := accountPassword(master, sk)
	id, err := d.openAs(ctx, password, d.keyHint(ctx, accountUser), accountUser)
	if err != nil {
		return nil, err
	}
	return &Account{d: d, master: master, password: password, keyID: id}, nil
}

// keyHint is the ID of ctdev's key labeled user as this computer's key lists
// it, or "" when it can't: restic then tries every key, each at the cost of
// a slow key derivation.
func (d *Destination) keyHint(ctx context.Context, user string) string {
	if d.computerKey == "" {
		return ""
	}
	keys, err := d.keysHinted(ctx, d.computerKey, d.computerKeyID)
	if err != nil {
		return ""
	}
	for _, k := range keys {
		if k.UserName == user && k.HostName == ctdevHost {
			return k.ID
		}
	}
	return ""
}

// current is the key password opens.
func (d *Destination) current(ctx context.Context, password, hint string) (resticKey, error) {
	keys, err := d.keysHinted(ctx, password, hint)
	if err != nil {
		return resticKey{}, err
	}
	for _, k := range keys {
		if k.Current {
			return k, nil
		}
	}
	return resticKey{}, errors.New("restic didn't say which key opened the repository")
}

// openAs opens the repository with password and returns the ID of the key it
// opened, which must be ctdev's key labeled user: a computer key's password
// typed as the master password, say, isn't the account key.
func (d *Destination) openAs(ctx context.Context, password, hint, user string) (string, error) {
	k, err := d.current(ctx, password, hint)
	if err != nil {
		return "", err
	}
	if k.UserName != user || k.HostName != ctdevHost {
		return "", ErrWrongPassword
	}
	return k.ID, nil
}

// replaceKeys swaps ctdev's key labeled user for one whose password is
// newPassword, opening the repository with password (whose key is hint). The
// order keeps a working key whatever fails: add the new key, check it opens
// the repository, run keep with its ID (saving what goes with it), and only
// then remove every other key labeled user. Up to the removal, a failure
// takes the new key back out, so nothing has changed. A failed removal
// leaves the new key and the old one both working, and returns the new
// key's ID with the error; any ID returned means the new key is in place.
func (d *Destination) replaceKeys(ctx context.Context, password, hint, user, newPassword string, keep func(newID string) error) (string, error) {
	newID, err := d.addKeyHinted(ctx, password, hint, user, ctdevHost, newPassword)
	if err != nil {
		return "", err
	}
	undo := func(err error) (string, error) {
		rm := run{password: password, keyHint: hint, args: []string{"key", "remove", "--retry-lock", retryLock, newID}}
		if _, rerr := d.restic(context.WithoutCancel(ctx), rm); rerr != nil {
			return "", fmt.Errorf("%w; removing the new key %s also failed (%v), but the old one still works", err, short(newID), rerr)
		}
		return "", err
	}
	if id, err := d.openAs(ctx, newPassword, newID, user); err != nil || id != newID {
		if err == nil {
			err = errors.New("the new key didn't open the repository")
		}
		return undo(fmt.Errorf("check the new key: %w", err))
	}
	if keep != nil {
		if err := keep(newID); err != nil {
			return undo(err)
		}
	}
	keys, err := d.keysHinted(ctx, newPassword, newID)
	if err == nil {
		for _, k := range keys {
			if k.UserName != user || k.HostName != ctdevHost || k.ID == newID {
				continue
			}
			rm := run{password: newPassword, keyHint: newID, args: []string{"key", "remove", "--retry-lock", retryLock, k.ID}}
			if _, err = d.restic(ctx, rm); err != nil {
				break
			}
		}
	}
	if err != nil {
		return newID, fmt.Errorf("the new key works, but the old one couldn't be removed and still works too; try again to remove it: %w", err)
	}
	return newID, nil
}

// Kit is the destination's Emergency Kit as this computer has it.
func (a *Account) Kit() string { return a.d.kit().String() }

// RemoveComputer removes a computer's key, so it can no longer open the
// destination. This computer's own key is refused with ErrThisComputer, and
// the account and Recovery Code keys aren't computers.
func (a *Account) RemoveComputer(ctx context.Context, c Computer) error {
	if c.ThisComputer || c.ID == a.d.computerKeyID {
		return ErrThisComputer
	}
	keys, err := a.d.keysHinted(ctx, a.password, a.keyID)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(keys, func(k resticKey) bool { return k.ID == c.ID })
	switch {
	case c.ID == "" || i < 0:
		return fmt.Errorf("%s has no key on %s any more", c.Hostname, a.d.Name)
	case keys[i].HostName == ctdevHost:
		return fmt.Errorf("key %s isn't a computer's", short(c.ID))
	}
	rm := run{password: a.password, keyHint: a.keyID, args: []string{"key", "remove", "--retry-lock", retryLock, c.ID}}
	if _, err := a.d.restic(ctx, rm); err != nil {
		return fmt.Errorf("remove %s's key: %w", c.Hostname, err)
	}
	return nil
}

// ChangeMasterPassword replaces the account key with one from the new master
// password and the same Secret Key, so the kit and every computer's key keep
// working.
func (a *Account) ChangeMasterPassword(ctx context.Context, master string) error {
	if master == "" {
		return errors.New("the master password can't be empty")
	}
	sk, err := parseSecretKey(a.d.secretKey)
	if err != nil {
		return err
	}
	password := accountPassword(master, sk)
	id, err := a.d.replaceKeys(ctx, a.password, a.keyID, accountUser, password, nil)
	if id != "" {
		a.master, a.password, a.keyID = master, password, id
	}
	return err
}

// RegenerateSecretKey replaces the Secret Key and the account key made from
// it and the master password, saves the new Secret Key on this computer, and
// returns the new kit. The old kit opens nothing once the old account key is
// removed. Every other computer's key keeps backing up; each pastes the new
// kit to unlock account actions there. A kit returned with an error is in
// force too: the new key works and only the old one's removal failed.
func (a *Account) RegenerateSecretKey(ctx context.Context) (string, error) {
	sk := newSecretKey()
	password := accountPassword(a.master, sk)
	old := a.d.secretKey
	id, err := a.d.replaceKeys(ctx, a.password, a.keyID, accountUser, password, func(string) error {
		a.d.secretKey = sk.String()
		if err := a.d.m.save(a.d); err != nil {
			a.d.secretKey = old
			return err
		}
		return nil
	})
	if id == "" {
		return "", err
	}
	a.password, a.keyID = password, id
	return a.Kit(), err
}

// SetCredentials replaces the destination's backend credentials on this
// computer, as after revoking a lost computer's at the provider. They're
// saved only once this computer's key opens the repository with them.
func (a *Account) SetCredentials(ctx context.Context, creds map[string]string) error {
	names := CredentialNames(a.d.Address)
	for k := range creds {
		if !slices.Contains(names, k) {
			return fmt.Errorf("a %s destination doesn't use %s", a.d.Backend(), k)
		}
	}
	for _, k := range names {
		if creds[k] == "" {
			return fmt.Errorf("%s is missing", k)
		}
	}
	next := *a.d
	next.credentials = maps.Clone(creds)
	if next.credentials == nil {
		next.credentials = map[string]string{}
	}
	if err := next.verifyComputerKey(ctx); err != nil {
		return err
	}
	if err := a.d.m.save(&next); err != nil {
		return err
	}
	a.d.credentials = next.credentials
	return nil
}

// verifyComputerKey checks this computer's own key opens the repository as d
// describes it.
func (d *Destination) verifyComputerKey(ctx context.Context) error {
	k, err := d.current(ctx, d.computerKey, d.computerKeyID)
	if err != nil {
		return err
	}
	if k.ID != d.computerKeyID {
		return fmt.Errorf("this computer's key %s didn't open the repository", short(d.computerKeyID))
	}
	return nil
}

// UpdateFromKit saves a pasted kit's address, credentials and Secret Key for
// a destination this computer is already set up on, as after a lost
// computer's flow or a new Secret Key elsewhere. It needs no master password,
// since this computer's own key is what opens the repository, and it checks
// that key opens it as the kit describes before saving anything.
func (m *Machine) UpdateFromKit(ctx context.Context, kitText string) (*Destination, error) {
	if m.Opts.DryRun {
		return nil, ErrDryRun
	}
	kit, err := ParseKit(kitText)
	if err != nil {
		return nil, err
	}
	if !m.hasDestination(kit.Destination) {
		return nil, fmt.Errorf("this computer isn't set up on %s: connect to it with the kit and master password instead", kit.Destination)
	}
	cur, err := m.Destination(kit.Destination)
	if err != nil {
		return nil, err
	}
	sk, err := parseSecretKey(kit.SecretKey)
	if err != nil {
		return nil, err
	}
	address, err := m.CheckAddress(kit.Address, "")
	if err != nil {
		return nil, err
	}
	next := *cur
	next.Address, next.credentials, next.secretKey = address, kit.Credentials, sk.String()
	if err := next.verifyComputerKey(ctx); err != nil {
		return nil, err
	}
	if err := m.save(&next); err != nil {
		return nil, err
	}
	return &next, nil
}

// hasDestination reports whether this computer has a destination saved
// under name.
func (m *Machine) hasDestination(name string) bool {
	if !destinationName.MatchString(name) {
		return false
	}
	_, err := os.Stat(m.destinationFile(name))
	return err == nil
}

// KitIsSetUp reports whether this computer is set up on the destination
// a pasted kit names, which decides between connecting and UpdateFromKit.
func (m *Machine) KitIsSetUp(kitText string) bool {
	kit, err := ParseKit(kitText)
	return err == nil && m.hasDestination(kit.Destination)
}

// Recovery is what a Recovery Code reset leaves: the destination, its new
// kit, and the new Recovery Code when one was asked for, each shown once.
type Recovery struct {
	Destination  *Destination
	Kit          string
	RecoveryCode string // "" unless a new one was made
}

// Recover resets a forgotten master password on a destination this computer
// is set up on: the Recovery Code opens the repository, and a new Secret Key
// and the new master password replace the account key. The new Secret Key is
// saved here. The used code keeps working unless newCode asks for a
// replacement. A Recovery returned with an error holds what is in force: the
// new account key, and a new code if it got that far.
func (d *Destination) Recover(ctx context.Context, code, master string, newCode bool) (Recovery, error) {
	return d.recover(ctx, code, master, newCode, func(string) error { return d.m.save(d) })
}

// RecoverFromKit is Recover from a pasted kit, offered when connecting
// fails. On a computer not set up on the destination, it then adds this
// computer's key and saves the destination, as connecting would have.
func (m *Machine) RecoverFromKit(ctx context.Context, kitText, code, master string, newCode bool) (Recovery, error) {
	if m.Opts.DryRun {
		return Recovery{}, ErrDryRun
	}
	kit, err := ParseKit(kitText)
	if err != nil {
		return Recovery{}, err
	}
	if m.hasDestination(kit.Destination) {
		d, err := m.Destination(kit.Destination)
		if err != nil {
			return Recovery{}, err
		}
		return d.Recover(ctx, code, master, newCode)
	}
	if err := m.checkNewName(kit.Destination); err != nil {
		return Recovery{}, err
	}
	address, err := m.CheckAddress(kit.Address, "")
	if err != nil {
		return Recovery{}, err
	}
	d := &Destination{Name: kit.Destination, Address: address, m: m, credentials: kit.Credentials}
	return d.recover(ctx, code, master, newCode, func(accountID string) error {
		// Opened with the new account key: d.secretKey is already the new one.
		sk, err := parseSecretKey(d.secretKey)
		if err != nil {
			return err
		}
		account := accountPassword(master, sk)
		d.computerKey = randomPassword()
		if d.computerKeyID, err = d.addKeyHinted(ctx, account, accountID, m.Username, m.Hostname, d.computerKey); err != nil {
			return err
		}
		if err := m.save(d); err != nil {
			rm := run{password: account, keyHint: accountID, args: []string{"key", "remove", "--retry-lock", retryLock, d.computerKeyID}}
			if _, rerr := d.restic(context.WithoutCancel(ctx), rm); rerr != nil {
				return fmt.Errorf("%w; removing the new key %s from the destination also failed: %v", err, short(d.computerKeyID), rerr)
			}
			return err
		}
		return nil
	})
}

// recover is the reset behind Recover and RecoverFromKit. keep saves the new
// Secret Key on this computer, given the new account key's ID; if it fails,
// the new account key is taken back out.
func (d *Destination) recover(ctx context.Context, code, master string, newCode bool, keep func(accountID string) error) (Recovery, error) {
	if d.m.Opts.DryRun {
		return Recovery{}, ErrDryRun
	}
	if master == "" {
		return Recovery{}, errors.New("the master password can't be empty")
	}
	codeID, err := d.openAs(ctx, code, d.keyHint(ctx, recoveryUser), recoveryUser)
	if err != nil {
		return Recovery{}, err
	}
	sk := newSecretKey()
	account := accountPassword(master, sk)
	old := d.secretKey
	accountID, err := d.replaceKeys(ctx, code, codeID, accountUser, account, func(id string) error {
		d.secretKey = sk.String()
		if err := keep(id); err != nil {
			d.secretKey = old
			return err
		}
		return nil
	})
	if accountID == "" {
		return Recovery{}, err
	}
	r := Recovery{Destination: d, Kit: d.kit().String()}
	if err != nil || !newCode {
		return r, err
	}
	next := newRecoveryCode()
	id, err := d.replaceKeys(ctx, account, accountID, recoveryUser, next, nil)
	if id != "" {
		r.RecoveryCode = next
	}
	if err != nil {
		return r, fmt.Errorf("make a new Recovery Code: %w", err)
	}
	return r, nil
}
