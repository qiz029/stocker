package engine

// ResolveAlias picks an instrument's display alias for one room,
// deterministically from the room's seed: the same (seed, instrumentID)
// pair always resolves to the same candidate, while different rooms may
// show different names for the same underlying stock. candidates is the
// full candidate set (convention: the primary alias is one of the
// entries); when it is empty the primary alias is used unchanged
// (pre-0006 data and simple fixtures have no candidates recorded).
func ResolveAlias(seed uint64, instrumentID, alias string, candidates []string) string {
	if len(candidates) == 0 {
		return alias
	}
	return candidates[Stream(seed, "alias", instrumentID).IntN(len(candidates))]
}
