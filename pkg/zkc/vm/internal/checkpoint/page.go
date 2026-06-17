package checkpoint

import "slices"

// Page represents a variable-length chunk of data within a given memory.
type Page[W any] struct {
	// Address specifies the physical address in memory where this page begins.
	address uint64
	// Data holds the words stored in this page, beginning at address.
	data []W
}

// NewPage constructs a single page of memory beginning at the given physical
// address and holding the given data.
func NewPage[W any](address uint64, data []W) Page[W] {
	return Page[W]{address, data}
}

// Clone returns a deep copy of this page, with its own copy of the underlying
// data.  This is useful when the page references storage (e.g. live memory)
// which may subsequently be mutated.
func (p Page[W]) Clone() Page[W] {
	return Page[W]{p.address, slices.Clone(p.data)}
}

// Address returns the physical address in memory where this page begins.
func (p Page[W]) Address() uint64 {
	return p.address
}

// Data returns the words stored in this page, beginning at Address.
func (p Page[W]) Data() []W {
	return p.data
}
