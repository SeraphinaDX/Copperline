package irc

import "testing"

func TestChannelHousekeepingNumericsAreHidden(t *testing.T) {
	hidden := []string{"315", "324", "329", "332", "333", "353", "366"}
	for _, command := range hidden {
		if !isChannelHousekeepingNumeric(command) {
			t.Errorf("numeric %s should be treated as channel housekeeping", command)
		}
	}

	visible := []string{"001", "301", "311", "367", "368", "473", "475", "477"}
	for _, command := range visible {
		if isChannelHousekeepingNumeric(command) {
			t.Errorf("numeric %s should remain visible", command)
		}
	}
}
