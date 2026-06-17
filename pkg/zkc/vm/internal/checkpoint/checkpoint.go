// Package checkpoint provides the data structures used to capture (a snapshot
// of) the state of an executing machine, so that execution can later be resumed
// from that point.  See CheckPoint for the central type and a discussion of how
// such snapshots may be optimised.
package checkpoint

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// CheckPoint represents a captured state of an executing machine, such that
// execution can be continued later from this position (sometimes also known as
// a "continuation").  As such, the checkpoint must include all information
// necessary to allow execution to continue.  However, to reduce the size of a
// checkpoint, certain optimsiations of this information are permitted.  As
// such, implementations can manage this information in different ways (e.g.
// using compression, read-access lists, page-access lists, etc).
//
// For example suppose that a machine, upon reaching the checkpoint, has 1GB of
// some RAM allocated.  A full checkpoint would include all of this data and,
// therefore, be at least 1GB in size.  Suppose, however, that executing the
// machine from this point onwards only requires accessing 1MB of data from that
// RAM.  Then, in fact, a valid implementation might only store that 1MB of
// information (e.g. to reduce network transport costs, or proof size, etc).
//
// Finally, to support more aggressive optimisation, checkpoints have a notion
// of their "validity window".   To understand this, consider a refinement of
// the example above.  Suppose there are M execution steps remaining from the
// checkpoint until the machine terminates, but that executing these M steps now
// requires the full 1GB of RAM.  However, we now suppose that executing the
// next N steps requires accessing only 1MB of that RAM. Then, the checkpoint
// may only store that 1MB of data if it is marked as being valid for (upto) N
// steps of execution.  The purpose of this is to allow a program's execution to
// be broken up into multiple checkpoints, such that each checkpoint only stores
// the data it requires for executing its portion of the overall execution.
type CheckPoint[W word.Word[W]] struct {
	// Call stack holding caller state for nested calls.
	callStack []StackFrame
	// Data stack holding the activation records (registers) of active function
	// calls.  The current frame begins at fp.
	dataStack []W
	// Snapshot of all mutable Memory.
	memory []Memory[W]
}

// NewCheckPoint constructs a checkpoint from a snapshot of the call stack, data
// stack and mutable memories of an executing machine.  The provided slices are
// retained by reference, so callers which intend to continue mutating the
// underlying storage (e.g. an interpreter's live stacks) should pass copies.
func NewCheckPoint[W word.Word[W]](callStack []StackFrame, dataStack []W, memory []Memory[W]) CheckPoint[W] {
	return CheckPoint[W]{callStack, dataStack, memory}
}

// CallStack returns the captured call stack of paused callers.
func (p CheckPoint[W]) CallStack() []StackFrame {
	return p.callStack
}

// DataStack returns the captured data stack holding the activation records
// (registers) of all active function calls.
func (p CheckPoint[W]) DataStack() []W {
	return p.dataStack
}

// Memories returns the captured snapshots of all mutable memories.
func (p CheckPoint[W]) Memories() []Memory[W] {
	return p.memory
}

// StackFrame captures relevant information about all functions currently
// executing a CALL.  Such functions are "paused" whilst the active function is
// being executed.  The purpose of a stack frame is to record the Frame Pointer
// (FP) and Program Counter (PC) of the relevant function so that these can be
// restored when it becomes the active function.
type StackFrame struct {
	// module identifier of the executing function.
	FunctionId uint
	// frame pointer of the executing function.
	FramePointer uint32
	// program counter identifies next bytecode to execute.
	ProgramCounter uint32
}

// NewStackFrame constructs a stack frame recording the function identifier,
// frame pointer and program counter of a paused function.
func NewStackFrame(fid uint, fp uint32, pc uint32) StackFrame {
	return StackFrame{fid, fp, pc}
}

// ============================================================================
// Encoding / Decoding
// ============================================================================
//
// A checkpoint is serialised into a flat byte sequence using a big-endian
// layout.  Counts, identifiers and addresses are written as fixed-width
// integers, whilst each word is written using exactly bytesPerWord(W) bytes
// (derived from the word type's Bandwidth).  The bytes-per-word is itself
// written as the first field, so that unmarshalling can sanity check that the
// data was produced for a word type of the expected width.

// MarshalBinary implements encoding.BinaryMarshaler, serialising this checkpoint
// into a flat big-endian byte sequence (see above).
func (p CheckPoint[W]) MarshalBinary() ([]byte, error) {
	var (
		buf   []byte
		nbits = bytesPerWord[W]()
		tmp   = make([]byte, nbits)
	)
	// Bytes-per-word: written first so it can be validated on unmarshalling.
	buf = binary.BigEndian.AppendUint32(buf, uint32(nbits))
	// Call stack: count followed by each frame.
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(p.callStack)))
	//
	for _, f := range p.callStack {
		buf = binary.BigEndian.AppendUint64(buf, uint64(f.FunctionId))
		buf = binary.BigEndian.AppendUint32(buf, f.FramePointer)
		buf = binary.BigEndian.AppendUint32(buf, f.ProgramCounter)
	}
	// Data stack: count followed by each word.
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(p.dataStack)))
	//
	for _, w := range p.dataStack {
		buf = append(buf, w.BigInt().FillBytes(tmp)...)
	}
	// Memories: count followed by each memory.
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(p.memory)))
	//
	for _, m := range p.memory {
		buf = binary.BigEndian.AppendUint64(buf, uint64(m.moduleId))
		buf = binary.BigEndian.AppendUint32(buf, uint32(len(m.pages)))
		//
		for _, pg := range m.pages {
			buf = binary.BigEndian.AppendUint64(buf, pg.address)
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(pg.data)))
			//
			for _, w := range pg.data {
				buf = append(buf, w.BigInt().FillBytes(tmp)...)
			}
		}
	}
	//
	return buf, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler, reconstructing a
// checkpoint previously serialised by MarshalBinary.
func (p *CheckPoint[W]) UnmarshalBinary(data []byte) error {
	var (
		r     = &reader{data: data}
		nbits = bytesPerWord[W]()
		err   error
		n     uint32
	)
	// Bytes-per-word: validate the data was produced for a word type of the
	// expected width before decoding anything else.
	if n, err = r.uint32(); err != nil {
		return err
	} else if n != uint32(nbits) {
		return fmt.Errorf("invalid checkpoint: expected %d bytes-per-word, got %d", nbits, n)
	}
	// Call stack.
	if n, err = r.uint32(); err != nil {
		return err
	}
	//
	callStack := make([]StackFrame, n)
	//
	for i := range callStack {
		fid, err := r.uint64()
		if err != nil {
			return err
		}
		//
		fp, err := r.uint32()
		if err != nil {
			return err
		}
		//
		pc, err := r.uint32()
		if err != nil {
			return err
		}
		//
		callStack[i] = NewStackFrame(uint(fid), fp, pc)
	}
	// Data stack.
	if n, err = r.uint32(); err != nil {
		return err
	}
	//
	dataStack := make([]W, n)
	//
	for i := range dataStack {
		if dataStack[i], err = readWord[W](r, nbits); err != nil {
			return err
		}
	}
	// Memories.
	if n, err = r.uint32(); err != nil {
		return err
	}
	//
	memory := make([]Memory[W], n)
	//
	for i := range memory {
		if memory[i], err = readMemory[W](r, nbits); err != nil {
			return err
		}
	}
	//
	p.callStack = callStack
	p.dataStack = dataStack
	p.memory = memory
	//
	return nil
}

// readMemory decodes a single memory snapshot (module identifier plus pages).
func readMemory[W word.Word[W]](r *reader, nbits int) (Memory[W], error) {
	var (
		mid, err = r.uint64()
		np       uint32
	)
	//
	if err != nil {
		return Memory[W]{}, err
	}
	//
	if np, err = r.uint32(); err != nil {
		return Memory[W]{}, err
	}
	//
	pages := make([]Page[W], np)
	//
	for j := range pages {
		if pages[j], err = readPage[W](r, nbits); err != nil {
			return Memory[W]{}, err
		}
	}
	//
	return Memory[W]{uint(mid), pages}, nil
}

// readPage decodes a single page (address plus data words).
func readPage[W word.Word[W]](r *reader, nbits int) (Page[W], error) {
	var (
		address, err = r.uint64()
		nd           uint32
	)
	//
	if err != nil {
		return Page[W]{}, err
	}
	//
	if nd, err = r.uint32(); err != nil {
		return Page[W]{}, err
	}
	//
	data := make([]W, nd)
	//
	for k := range data {
		if data[k], err = readWord[W](r, nbits); err != nil {
			return Page[W]{}, err
		}
	}
	//
	return Page[W]{address, data}, nil
}

// readWord decodes a single word from the next nbits bytes (big-endian).
func readWord[W word.Word[W]](r *reader, nbits int) (W, error) {
	var (
		zero    W
		bs, err = r.bytes(nbits)
	)
	//
	if err != nil {
		return zero, err
	}
	//
	var value big.Int
	value.SetBytes(bs)
	//
	return zero.SetBigInt(&value), nil
}

// bytesPerWord returns the number of bytes required to hold a single word of
// type W, derived from the word type's Bandwidth (rounded up to whole bytes).
func bytesPerWord[W word.Word[W]]() int {
	var zero W
	return int((zero.Bandwidth() + 7) / 8)
}

// reader is a minimal cursor over a byte slice used during decoding.  Each read
// advances the cursor, returning io.ErrUnexpectedEOF if insufficient bytes
// remain.
type reader struct {
	data []byte
	pos  int
}

// bytes returns the next n bytes (a sub-slice of the underlying data).
func (r *reader) bytes(n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, io.ErrUnexpectedEOF
	}
	//
	bs := r.data[r.pos : r.pos+n]
	r.pos += n
	//
	return bs, nil
}

// uint32 reads the next big-endian uint32.
func (r *reader) uint32() (uint32, error) {
	bs, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	//
	return binary.BigEndian.Uint32(bs), nil
}

// uint64 reads the next big-endian uint64.
func (r *reader) uint64() (uint64, error) {
	bs, err := r.bytes(8)
	if err != nil {
		return 0, err
	}
	//
	return binary.BigEndian.Uint64(bs), nil
}
