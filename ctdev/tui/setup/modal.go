package setup

import (
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

// ModalModel handles the info modal overlay.
type ModalModel struct {
	state  *s.SettingState
	mode   Mode
	closed bool
}

func NewModal(state *s.SettingState, mode Mode) ModalModel {
	return ModalModel{state: state, mode: mode}
}

func (inst *ModalModel) Closed() bool { return inst.closed }
