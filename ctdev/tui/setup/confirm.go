package setup

import (
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
)

// ConfirmModel handles the confirmation screen before applying changes.
type ConfirmModel struct {
	changes   []changeEntry
	confirmed bool
	cancelled bool
	dryRun    bool
}

type changeEntry struct {
	name string
	from string
	to   string
}

func NewConfirm(states []s.SettingState, dryRun bool) ConfirmModel {
	var changes []changeEntry
	for _, st := range states {
		if st.NeedsApply(false) {
			changes = append(changes, changeEntry{name: st.Setting.Name, from: st.CurrentValue, to: st.DesiredValue})
		}
	}
	return ConfirmModel{changes: changes, dryRun: dryRun}
}

func (inst *ConfirmModel) Confirmed() bool { return inst.confirmed }
func (inst *ConfirmModel) Cancelled() bool { return inst.cancelled }
