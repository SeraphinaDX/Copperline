package relay

import (
	"testing"

	ircclient "copperline/internal/irc"
)

func TestCloneSnapshotIsIndependent(t *testing.T) {
	in := ircclient.StateSnapshot{Servers: []ircclient.ServerSnapshot{{
		Name:         "test",
		Capabilities: []string{"message-tags"},
		Targets: []ircclient.TargetSnapshot{{
			Target: "#test",
			Users:  []ircclient.UserSnapshot{{Nick: "alice", Prefix: "@"}},
		}},
	}}}
	out := cloneSnapshot(in)
	out.Servers[0].Capabilities[0] = "changed"
	out.Servers[0].Targets[0].Users[0].Nick = "bob"
	if in.Servers[0].Capabilities[0] != "message-tags" || in.Servers[0].Targets[0].Users[0].Nick != "alice" {
		t.Fatal("cloneSnapshot shares mutable slices with its input")
	}
}
