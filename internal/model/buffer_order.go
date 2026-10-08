package model

import (
	"sort"
	"strings"
)

// SortBuffersAlphabetically orders a UI snapshot by case-insensitive target
// name within each network, with its status buffer first. Network order follows
// first appearance. Sorting a SnapshotInfo result never changes State.Order.
func SortBuffersAlphabetically(buffers []BufferInfo) {
	networks := make(map[string]int)
	names := make(map[string]string)
	for _, b := range buffers {
		if _, exists := networks[b.Server]; !exists {
			networks[b.Server] = len(networks)
		}
		names[b.Target] = strings.ToLower(b.Target)
	}
	sort.SliceStable(buffers, func(i, j int) bool {
		a, b := buffers[i], buffers[j]
		if a.Server != b.Server {
			return networks[a.Server] < networks[b.Server]
		}
		if (a.Target == "*server*") != (b.Target == "*server*") {
			return a.Target == "*server*"
		}
		return names[a.Target] < names[b.Target]
	})
}
