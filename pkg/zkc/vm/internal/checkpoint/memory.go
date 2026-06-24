package checkpoint

// Memory captures a snapshot of the contents of a single (mutable) memory
// module.  To support sparse memories, the contents are described as a sequence
// of pages, each covering a contiguous region; regions not covered by any page
// are implicitly zero.
type Memory[W any] struct {
	// Module identifier for this memory.
	moduleId uint16
	// Pages determines the contents of the given memory in this snapshot.
	pages []Page[W]
}

// NewMemory constructs a snapshot of a single memory module, identified by its
// module identifier and described by the given sequence of pages.
func NewMemory[W any](moduleId uint16, pages []Page[W]) Memory[W] {
	return Memory[W]{moduleId, pages}
}

// ModuleId returns the module identifier of the memory captured by this
// snapshot.
func (p Memory[W]) ModuleId() uint16 {
	return p.moduleId
}

// Pages returns the pages describing the captured contents of this memory.
func (p Memory[W]) Pages() []Page[W] {
	return p.pages
}
