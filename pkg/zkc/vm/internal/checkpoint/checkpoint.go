// Package checkpoint provides the data structures used to capture (a snapshot
// of) the state of an executing machine, so that execution can later be resumed
// from that point.  See CheckPoint for the central type and a discussion of how
// such snapshots may be optimised.
package checkpoint

import (
	"bytes"
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
	// Snapshot of all checkpointed memories.
	memory []Memory[W]
}

// NewCheckPoint constructs a checkpoint from a snapshot of the call stack, data
// stack and memories of an executing machine.  The provided slices are
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

// Memories returns the captured memory snapshots.
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
// A checkpoint is serialised into a flat byte sequence beginning with
// checkpointHeader.  Counts, identifiers and addresses are written as fixed-
// width big-endian integers, whilst each word is written as an unsigned
// LEB128-style variable-length integer.

var checkpointHeader = []byte{'Z', 'K', 'C', 'P', 1}

// MarshalBinary implements encoding.BinaryMarshaler, serialising this checkpoint
// into the checkpoint format (see above).
func (p CheckPoint[W]) MarshalBinary() ([]byte, error) {
	buf := append([]byte{}, checkpointHeader...)
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
		var err error
		//
		if buf, err = appendUvarWord(buf, w); err != nil {
			return nil, err
		}
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
				var err error
				//
				if buf, err = appendUvarWord(buf, w); err != nil {
					return nil, err
				}
			}
		}
	}
	//
	return buf, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler, reconstructing a
// checkpoint previously serialised by MarshalBinary.
func (p *CheckPoint[W]) UnmarshalBinary(data []byte) error {
	if !bytes.HasPrefix(data, checkpointHeader) {
		return fmt.Errorf("invalid checkpoint: missing header")
	}
	//
	data = data[len(checkpointHeader):]
	//
	return p.unmarshalFromReader(&reader{data: data}, readUvarWord[W])
}

// unmarshalFromReader decodes the checkpoint payload.  The supplied readWord
// function handles the word representation.
func (p *CheckPoint[W]) unmarshalFromReader(r *reader, readWord func(*reader) (W, error)) error {
	var (
		err error
		n   uint32
	)
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
		if dataStack[i], err = readWord(r); err != nil {
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
		if memory[i], err = readMemory(r, readWord); err != nil {
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
func readMemory[W word.Word[W]](r *reader, readWord func(*reader) (W, error)) (Memory[W], error) {
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
		if pages[j], err = readPage[W](r, readWord); err != nil {
			return Memory[W]{}, err
		}
	}
	//
	return Memory[W]{uint(mid), pages}, nil
}

// readPage decodes a single page (address plus data words).
func readPage[W word.Word[W]](r *reader, readWord func(*reader) (W, error)) (Page[W], error) {
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
		if data[k], err = readWord(r); err != nil {
			return Page[W]{}, err
		}
	}
	//
	return Page[W]{address, data}, nil
}

// appendUvarWord appends w using unsigned LEB128-style variable-length
// encoding.
func appendUvarWord[W word.Word[W]](buf []byte, w W) ([]byte, error) {
	value := w.BigInt()
	//
	if value.Sign() < 0 {
		return nil, fmt.Errorf("invalid checkpoint word: negative value 0x%s", value.Text(16))
	}
	//
	return appendUvarBig(buf, value), nil
}

// appendUvarBig appends value using unsigned LEB128-style variable-length
// encoding.  The input is copied before shifting, so callers retain ownership of
// value.
func appendUvarBig(buf []byte, value *big.Int) []byte {
	if value.Sign() == 0 {
		return append(buf, 0)
	}
	//
	var v big.Int
	v.Set(value)
	//
	for v.Sign() > 0 {
		b := byte(v.Uint64() & 0x7f)
		v.Rsh(&v, 7)
		//
		if v.Sign() != 0 {
			b |= 0x80
		}
		//
		buf = append(buf, b)
	}
	//
	return buf
}

// readUvarWord decodes a single unsigned LEB128-style word.
func readUvarWord[W word.Word[W]](r *reader) (W, error) {
	var (
		zero      W
		value     big.Int
		shift     uint
		bandwidth = zero.Bandwidth()
		maxBytes  = maxUvarWordBytes(bandwidth)
	)
	//
	for nbytes := uint(1); ; nbytes++ {
		if maxBytes != 0 && nbytes > maxBytes {
			return zero, fmt.Errorf("invalid checkpoint: variable-length word exceeds %d-bit bandwidth", bandwidth)
		}
		//
		b, err := r.byte()
		if err != nil {
			return zero, err
		}
		//
		if payload := b & 0x7f; payload != 0 {
			var part big.Int
			part.SetUint64(uint64(payload))
			part.Lsh(&part, shift)
			value.Or(&value, &part)
		}
		//
		if b&0x80 == 0 {
			break
		}
		//
		shift += 7
	}
	//
	if err := validateWordBandwidth[W](&value); err != nil {
		return zero, err
	}
	//
	return zero.SetBigInt(&value), nil
}

// validateWordBandwidth reports an error when value does not fit into W.
func validateWordBandwidth[W word.Word[W]](value *big.Int) error {
	var zero W
	//
	if bandwidth := zero.Bandwidth(); bandwidth != ^uint(0) && uint(value.BitLen()) > bandwidth {
		return fmt.Errorf("invalid checkpoint: word value 0x%s exceeds %d-bit bandwidth", value.Text(16), bandwidth)
	}
	//
	return nil
}

// maxUvarWordBytes returns the maximum canonical byte length for a word with a
// finite bit bandwidth.  A zero result means unbounded.
func maxUvarWordBytes(bandwidth uint) uint {
	if bandwidth == ^uint(0) {
		return 0
	}
	//
	return (bandwidth + 6) / 7
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

// byte returns the next byte.
func (r *reader) byte() (byte, error) {
	bs, err := r.bytes(1)
	if err != nil {
		return 0, err
	}
	//
	return bs[0], nil
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
