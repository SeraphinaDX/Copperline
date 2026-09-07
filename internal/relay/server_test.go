package relay

import "testing"

func TestRelayHousekeepingNumericsAreNotForwarded(t *testing.T) {
	hidden := []string{"315", "324", "328", "329", "332", "333", "352", "353", "354", "366"}
	for _, command := range hidden {
		if !relayHousekeepingNumeric(command) {
			t.Errorf("numeric %s should be suppressed from raw relay event forwarding", command)
		}
	}
	for _, command := range []string{"001", "311", "401", "PRIVMSG", "CONNECTED", "DISCONNECTED"} {
		if relayHousekeepingNumeric(command) {
			t.Errorf("event %s should still be forwarded by the relay", command)
		}
	}
}
