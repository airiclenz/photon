package tune

// history is the storyboard's versions in order, the first as loaded, and the one in effect.
type history struct {
	versions [][]byte
	at       int
}

// newHistory is a history holding src alone.
func newHistory(src []byte) history {
	return history{versions: [][]byte{src}}
}

// current returns the version in effect.
func (h *history) current() []byte { return h.versions[h.at] }

// push makes src the version in effect, dropping the versions undone before it.
func (h *history) push(src []byte) {
	h.versions = append(h.versions[:h.at+1], src)
	h.at++
}

// canUndo says there is a version before the one in effect.
func (h *history) canUndo() bool { return h.at > 0 }

// canRedo says an undone version follows the one in effect.
func (h *history) canRedo() bool { return h.at < len(h.versions)-1 }
