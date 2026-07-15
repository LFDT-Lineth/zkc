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
	// clock is the memory's access clock at snapshot time (the timestamp of its
	// most recent access); zero for memories which do not track timestamps.
	// Restored so that accesses after a resume stay monotonic.
	clock uint64
}

// NewMemory constructs a snapshot of a single memory module, identified by its
// module identifier, described by the given sequence of pages, and with the
// given access clock (zero for memories which do not track timestamps).
func NewMemory[W any](moduleId uint16, pages []Page[W], clock uint64) Memory[W] {
	return Memory[W]{moduleId, pages, clock}
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

// Clock returns the memory's access clock at snapshot time.
func (p Memory[W]) Clock() uint64 {
	return p.clock
}
