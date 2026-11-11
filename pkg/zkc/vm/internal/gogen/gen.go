// Copyright Consensys Incorporated
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// Package gogen compiles a ZkC bytecode program into native Go source: the
// "fast execution mode" alternative to interpreting the program.
//
// The generator consumes a program descriptor over word.Uint — the same
// program the reference executor interprets — walking its bytecode vectors
// directly (see pkg/zkc/vm/internal/bytecode):
//
//   - Arithmetic is EXACT (word.Uint is unbounded): there is no accumulator
//     overflow.  All width enforcement happens at store time, where a single
//     register store checks the declared bit width and a multi-register store
//     distributes the value across the limbs (lowest register = least
//     significant) and fails only on bits beyond the total width.  Explicit
//     CHECKCAST bytecodes are emitted as the same width checks (and, like
//     store checks, are elided when the bound analysis proves them dead).
//   - The generator picks a Go representation per result from a static bound
//     derived from register widths: plain uint64 when the bound fits 64 bits,
//     a lo/hi pair (math/bits) up to 128 bits, and a clean "unsupported" error
//     beyond that.  The same bounds prove most store-width checks dead, so
//     they are simply not emitted.
//   - Failures (width checks, underflow, division by zero, FAIL) panic with a
//     `failure` value recovered once in Run, so generated functions have plain
//     value signatures with no error plumbing.
//   - The 2-D program counter (macro vector + micro code) becomes labelled Go:
//     skips and jumps are gotos between labelled positions.
//   - Functions become Go functions (the Go stack is the call stack); shared
//     memories become package-level globals.
//
// Registers and constants up to 128 bits are supported via the lo/hi pair
// representation (including wide limbs inside a multi-register store, as the
// bitwise-lowering helpers produce).  A native (field-element) register is a
// single uint64 local when the field modulus fits 64 bits (a field value fits
// uint64; the mod-P emitters reduce operands on use, so a value at rest need
// not be reduced — as in the interpreter).  Registers/constants beyond 128
// bits, native registers under a modulus wider than 64 bits, and moduli beyond
// 64 bits for mod-P ops are rejected with descriptive errors; callers treat
// these programs as out of scope, not failures.
package gogen

import (
	"fmt"
	"go/format"
	"math/big"
	"regexp"
	"slices"
	"strings"

	lword "github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// regId identifies a register within the enclosing function's frame.
type regId = bytecode.RegisterId

type descFunction = descriptor.Function[word.Uint]

// BytecodeVector is the body of a function: a slice of (vectorised) bytecode vectors.
type BytecodeVector = []bytecode.Vector[word.Uint]

type memRole int

const (
	romInput     memRole = iota // non-static read-only: loaded from the program inputs
	womOutput                   // write-once: forms the program outputs (grow-on-write)
	sromStatic                  // static read-only: fixed contents baked into the program
	ramScratch                  // read-write scratch: zero-initialised, grows on write
	pagedScratch                // paged read-write scratch (demand-allocated pages)
)

type memInfo struct {
	name string
	// id is the module identifier of this memory within the program.
	id      uint16
	varName string
	role    memRole
	geom    descriptor.Memory[word.Uint]
	// dataWidths are the bit-widths of the memory's data lines, in order; the
	// main harness bakes these in to pack/unpack the byte encoding shared with
	// `zkc exec` (see encodeBytes / decodeBytes).
	dataWidths []uint
	// contents holds the baked initial values of a static (SROM) memory; nil
	// for all other roles.
	contents []uint64
	// elemType is the Go array element type of a static (SROM) memory (the
	// narrowest that holds its contents); empty for all other roles.
	elemType string
}

// pagedPageSize is the number of cells per page of a paged scratch memory in
// the generated program (mirroring interpreter.PAGE_SIZE).
const pagedPageSize uint64 = 1 << 20

// pos is a 2-D program-counter position: a macro vector index plus a micro code
// index within that vector.  It is the target of skips and jumps.
type pos struct {
	macro uint
	micro uint
}

// Generate compiles a bytecode program into a self-contained Go source file.
// See the package documentation for the semantics contract and Config for the
// artefact shape.
func Generate(program descriptor.Program[word.Uint], cfg Config) (string, error) {
	if cfg.Package == "" {
		cfg.Package = "main"
	}

	g := &generator{
		config:   cfg,
		memByID:  map[uint]memInfo{},
		funcByID: map[uint]*descFunction{},
		program:  program,
		modulus:  program.Field().Modulus(),
		names:    map[string]bool{},
		helpers:  map[string]bool{},
	}
	mainID, hasMain := uint(0), false

	for id, m := range program.Modules() {
		switch mm := m.(type) {
		case *descriptor.Memory[word.Uint]:
			info, err := g.classifyMemory(mm)
			if err != nil {
				return "", err
			}

			info.id = uint16(id)
			g.memByID[uint(id)] = info

			switch info.role {
			case romInput:
				g.inputs = append(g.inputs, info)
			case womOutput:
				g.outputs = append(g.outputs, info)
			case sromStatic:
				g.sroms = append(g.sroms, info)
			case ramScratch:
				g.rams = append(g.rams, info)
			case pagedScratch:
				g.pageds = append(g.pageds, info)
			}
		case *descFunction:
			g.funcByID[uint(id)] = mm
			if mm.Name() == "main" {
				mainID, hasMain = uint(id), true
			}
		default:
			return "", fmt.Errorf("gogen: unsupported module %q (%T)", m.Name(), m)
		}
	}

	if !hasMain {
		return "", fmt.Errorf("gogen: no 'main' function found")
	}
	// The package-main JSON harness encodes inputs/outputs as bytes (the format
	// `zkc exec` accepts), which a field-element data line has no fixed width
	// for.  Non-main packages have no harness, so only main is affected.
	if g.config.Package == "main" {
		for _, info := range append(append([]memInfo{}, g.inputs...), g.outputs...) {
			if hasNativeDataLine(info) {
				return "", fmt.Errorf("gogen: memory %q has a field-element data line, which the JSON "+
					"harness cannot encode", info.name)
			}
		}
	}
	// Checkpointing requires a known shard function, and that every paged
	// memory's pages are row aligned, as checkpoint pages are addressed by row
	// (see emitCheckpointWriter).
	if g.config.CheckPointing.HasValue() {
		if fn := g.config.CheckPointing.Unwrap().ShardFunction; !hasFunction(g.funcByID, fn) {
			return "", fmt.Errorf("gogen: unknown shard function %q", fn)
		}
		//
		for _, info := range g.pageds {
			if lanes := uint64(len(info.geom.DataRegisters())); pagedPageSize%lanes != 0 {
				return "", fmt.Errorf("gogen: paged memory %q has %d data lanes, which do not divide the page size %d",
					info.name, lanes, pagedPageSize)
			}
		}
	}
	// Only emit functions reachable from main (transitively via CALL); this keeps
	// generation scoped to the program actually being executed and avoids failing
	// on unrelated helper functions that use out-of-scope instructions.
	order := g.reachableFunctions(mainID)
	// Emit each function body first so helper/import requirements are known
	// before the imports.
	bodies := map[uint]*code{}

	for _, id := range order {
		fn := g.funcByID[id]
		if fn.IsNative() {
			return "", fmt.Errorf("gogen: native function %q unsupported", fn.Name())
		}

		var b code
		if err := g.emitFunction(&b, id, fn, cfg); err != nil {
			return "", err
		}

		bodies[id] = &b
	}
	// Assemble the full file.
	var file code
	if err := g.emitFile(&file, order, bodies); err != nil {
		return "", err
	}

	return formatSource(file.String())
}

type generator struct {
	config   Config
	memByID  map[uint]memInfo
	inputs   []memInfo
	outputs  []memInfo
	sroms    []memInfo // static read-only memories (baked contents)
	rams     []memInfo // read-write scratch memories
	pageds   []memInfo // paged read-write scratch memories
	program  descriptor.Program[word.Uint]
	funcByID map[uint]*descFunction
	modulus  *big.Int        // the machine's prime modulus (for mod-P ops)
	names    map[string]bool // sanitized identifiers already taken
	helpers  map[string]bool // optional generated helpers/import needs
	usesBits bool            // whether math/bits is referenced (decides the import)
	usesModP fieldHelpers    // which mod-P helpers are referenced
	cur      fnView          // the function currently being emitted (return shape)
	iv       *intervals      // bound analysis for the function being emitted
}

const (
	helperCatU64     = "catU64"
	helperChr        = "chr"
	helperDbgB       = "dbgB"
	helperDbgU       = "dbgU"
	helperDbgWriter  = "dbgw"
	helperDone       = "done"
	helperFmtSprintf = "fmt.Sprintf"
	helperShl128     = "shl128"
	helperShr128     = "shr128"
	helperU128       = "u128"
	helperWideInt    = "wideInt"
	helperWideShl    = "wideShl"
	helperWideShr    = "wideShr"
)

func (g *generator) useHelper(name string) {
	g.helpers[name] = true
}

func (g *generator) usesHelper(name string) bool {
	return g.helpers[name]
}

func cloneHelpers(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for name, used := range src {
		if used {
			dst[name] = true
		}
	}

	return dst
}

// fnView captures the parts of the function currently being emitted that the
// per-instruction emitters need: how to spell a RETURN.
type fnView struct {
	isBoot   bool     // the boot frame ('main'); its outputs are discarded
	outNames []string // output result names, limb-expanded (empty for boot)
	tailLoop bool     // body contains a self tail call, needing the entry label
}

// hasFunction checks whether a function with the given name exists.
func hasFunction(funcs map[uint]*descFunction, name string) bool {
	for _, fn := range funcs {
		if fn.Name() == name {
			return true
		}
	}

	return false
}

// reachableFunctions returns the ids of the functions reachable from the entry
// (transitively via CALL), with the entry first and the rest in ascending id
// order for deterministic output.
func (g *generator) reachableFunctions(entry uint) []uint {
	seen := map[uint]bool{entry: true}
	worklist := []uint{entry}

	var rest []uint

	for len(worklist) > 0 {
		id := worklist[0]
		worklist = worklist[1:]

		fn := g.funcByID[id]
		for _, vec := range fn.Vectors() {
			for _, insn := range vec.Bytecodes {
				// Both regular and unconditional (range-check) calls reach a callee.
				call, ok := insn.(*bytecode.Call[word.Uint])
				if !ok {
					continue
				}
				//
				callee := uint(call.Target)
				if !seen[callee] {
					seen[callee] = true
					worklist = append(worklist, callee)
					rest = append(rest, callee)
				}
			}
		}
	}

	slices.Sort(rest)

	return append([]uint{entry}, rest...)
}

// ===========================================================================
// code: a tiny line-oriented Go source buffer
// ===========================================================================

// code accumulates generated Go line by line.  It intentionally does NOT manage
// indentation — formatSource (gofmt) reformats the result — which keeps the
// generation logic free of whitespace bookkeeping.
type code struct{ b strings.Builder }

// line appends a single line of source.
func (c *code) line(s string) { c.b.WriteString(s); c.b.WriteByte('\n') }

// linef appends a single formatted line of source.
func (c *code) linef(format string, a ...any) { fmt.Fprintf(&c.b, format, a...); c.b.WriteByte('\n') }

// commentf appends a sanitized line comment.
func (c *code) commentf(format string, a ...any) {
	if text := commentText(fmt.Sprintf(format, a...)); text != "" {
		c.linef("// %s", text)
	}
}

// raw appends already-rendered source verbatim (no trailing newline added).
func (c *code) raw(s string) { c.b.WriteString(s) }

// block emits `{ <body> }`, scoping any temporaries the body declares (and
// letting a forward goto legally jump over the whole block).
func (c *code) block(body func()) {
	c.line("{")
	body()
	c.line("}")
}

func (c *code) String() string { return c.b.String() }

// ===========================================================================
// File scaffold
// ===========================================================================

// emitFile writes the package, imports, failure helpers, memory globals and
// helpers, the Run entry point, every reachable function, and (for package
// main) the JSON stdin/stdout harness.
func (g *generator) emitFile(c *code, order []uint, bodies map[uint]*code) error {
	c.line("// Code generated by zkc gogen. DO NOT EDIT.")

	if g.config.Source != "" {
		c.linef("// Source: %s", g.config.Source)
	}

	c.linef("package %s", g.config.Package)
	c.line("")
	g.emitImports(c)

	// Memories are shared across the call stack, so they live as package-level
	// globals (matching the VM's shared memory banks); callees read/write them
	// without threading slices through every signature.  Run resets them, so
	// the package is single-execution-at-a-time (like the VM itself).
	for _, m := range g.inputs {
		c.linef("// Input ROM %q.", m.name)
		c.linef("var %s []uint64", m.varName)
	}

	for _, m := range g.outputs {
		c.linef("// Output WOM %q.", m.name)
		c.linef("var %s []uint64", m.varName)
	}

	for _, m := range g.rams {
		c.linef("// Scratch RAM %q.", m.name)
		c.linef("var %s []uint64", m.varName)
		c.linef("var %s_stamp uint64", m.varName)
	}

	for _, m := range g.pageds {
		c.linef("// Paged RAM %q.", m.name)
		c.linef("var %s [][]uint64", m.varName)
		c.linef("var %s_stamp uint64", m.varName)
	}

	// Static read-only memories have fixed contents baked in as a literal.
	for _, m := range g.sroms {
		c.linef("// Static ROM %q.", m.name)
		c.linef("var %s = %s", m.varName, sromLiteral(m))
	}

	c.line("")

	for _, id := range order {
		c.raw(bodies[id].String())
	}

	if g.config.Package == "main" {
		if err := g.emitMainHarness(c); err != nil {
			return err
		}
		//
		g.emitConversionHelpers(c)
	}
	//
	emitTerminationHelpers(c, g.usesHelper(helperDone))
	//
	g.emitPrintfHelpers(c)
	//
	g.emitMemHelpers(c)
	g.emitShiftHelpers(c)
	g.emitWideIntHelper(c)
	g.emitModPHelpers(c)
	//
	return nil
}

func (g *generator) emitImports(c *code) {
	deps := []string{}
	if g.config.Package == "main" {
		deps = append(deps,
			`"bufio"`, `"encoding/hex"`, `"encoding/json"`, `"fmt"`, `"math/big"`, `"os"`, `"strconv"`, `"strings"`)
	}
	// The checkpoint writer encodes with encoding/binary.
	if g.config.Package == "main" && g.config.CheckPointing.HasValue() {
		deps = append(deps, `"encoding/binary"`)
	}

	if g.usesBits {
		deps = append(deps, `"math/bits"`)
	}
	// memWrite (WOM/dynamic RAM) and paged.set both use slices.Grow.
	if len(g.outputs) > 0 || len(g.rams) > 0 || len(g.pageds) > 0 {
		deps = append(deps, `"slices"`)
	}
	// Optional generated helpers add their own imports. Package main already
	// imports bufio/os/fmt/math/big for its harness
	if g.config.Package != "main" {
		if g.usesHelper(helperDbgWriter) {
			deps = append(deps, `"bufio"`, `"os"`)
		}

		// romRead and womWrite format their messages.
		if g.usesHelper(helperFmtSprintf) {
			deps = append(deps, `"fmt"`)
		}

		if g.usesHelper(helperDbgB) || g.usesHelper(helperU128) ||
			g.usesHelper(helperCatU64) || g.usesHelper(helperWideInt) {
			deps = append(deps, `"math/big"`)
		}

		if g.usesHelper(helperDbgU) {
			deps = append(deps, `"strconv"`)
		}
	}

	switch len(deps) {
	case 0:
		return
	case 1:
		c.linef("import %s", deps[0])
	default:
		slices.Sort(deps)
		c.line("import (")

		for _, d := range deps {
			c.line(d)
		}

		c.line(")")
	}

	c.line("")
}

// emitMemHelpers writes the grow-on-write / zero-on-miss helpers backing the
// writable memories, each only when some memory of that kind exists.
func (g *generator) emitMemHelpers(c *code) {
	var usesMemory = len(g.inputs) > 0 || len(g.outputs) > 0 || len(g.rams) > 0 || len(g.pageds) > 0

	if usesMemory {
		c.commentf("===============================================================================")
		c.commentf("Memory Helpers")
		c.commentf("===============================================================================")
		// roms
		if len(g.inputs) > 0 {
			c.line("")
			// An out-of-bounds read is a bug in the program (or its inputs)
			// rather than a rejected trace, hence (as for the interpreter's
			// StaticArray) it panics rather than fails.
			c.line("func romRead(data []uint64, addr uint64) uint64 {")
			c.line("if addr >= uint64(len(data)) {")
			c.line(`fail("out-of-bounds read of read-only memory")`)
			c.line("}")
			c.line("return data[addr]")
			c.line("}")
		}
		// woms
		if len(g.outputs) > 0 {
			c.line("")
			c.line("func womWrite(data []uint64, addr uint64, v uint64) []uint64 {")
			c.line("if addr < uint64(len(data)) && data[addr] != v {")
			c.line("fail(\"address already written for write-once memory\")")
			c.line("} else if n := addr + 1; uint64(len(data)) < n {")
			c.line("data = slices.Grow(data, int(n)-len(data))[:n]")
			c.line("}")
			c.line("data[addr] = v")
			c.line("return data")
			c.line("}")
		}
		// rams
		if len(g.rams) > 0 {
			c.line("")
			c.line("func ramRead(data []uint64, addr uint64) uint64 {")
			c.line("if addr < uint64(len(data)) {")
			c.line("return data[addr]")
			c.line("}")
			c.line("return 0")
			c.line("}")
			c.line("")
			//
			c.line("func ramWrite(data []uint64, addr uint64, v uint64) []uint64 {")
			c.line("if n := addr + 1; uint64(len(data)) < n {")
			c.line("data = slices.Grow(data, int(n)-len(data))[:n]")
			c.line("}")
			c.line("data[addr] = v")
			c.line("return data")
			c.line("}")
		}
		// paged rams
		if len(g.pageds) > 0 {
			c.line("")
			c.linef("const pageSize = %d", pagedPageSize)
			c.line("")
			c.line("func pagedRamRead(pages [][]uint64, addr uint64) uint64 {")
			c.line("page, off := addr/pageSize, addr%pageSize")
			c.line("if page < uint64(len(pages)) && pages[page] != nil {")
			c.line("return pages[page][off]")
			c.line("}")
			c.line("return 0")
			c.line("}")
			c.line("")
			c.line("func pagedRamWrite(pages [][]uint64, addr uint64, v uint64) [][]uint64 {")
			c.line("page, off := addr/pageSize, addr%pageSize")
			c.line("if n := page + 1; uint64(len(pages)) < n {")
			c.line("pages = slices.Grow(pages, int(n)-len(pages))[:n]")
			c.line("}")
			c.line("if pages[page] == nil {")
			c.line("pages[page] = make([]uint64, pageSize)")
			c.line("}")
			c.line("pages[page][off] = v")
			c.line("return pages")
			c.line("}")
		}
	}
}

// conversionHelpers are the static (machine-independent) helpers backing the
// main harness: a parser for the input encoding and the byte<->cell packing
// shared with `zkc exec`.  They are emitted only for package main.
const conversionHelpers = `// parseInput decodes an input memory's encoding: a hex string ("0x…", with "_"
// separators allowed) or a decimal literal — the same encoding ParseJsonInputFile
// accepts, so a file feeding "zkc exec" feeds this program unchanged.
func parseInput(s string) ([]byte, uint) {
	s = strings.ReplaceAll(s, "_", "")
	switch {
	case strings.HasPrefix(s, "0x"):
		return parseExtendedHexString(s[2:])
	case s == "":
		return nil, 0
	default:
		var v big.Int
		if _, ok := v.SetString(s, 10); !ok {
			fmt.Fprintln(os.Stderr, "malformed numeric literal: ", s)
			os.Exit(2)
		}
		bytes := v.Bytes()
		return bytes, uint(len(bytes)) * 8
	}
}

// hexString renders nbits of data in the extended hex format (the inverse of
// parseExtendedHexString).
func hexString(data []byte, nbits uint) string {
	if tail := nbits % 8; tail != 0 {
		return fmt.Sprintf("0x%s~%d", hex.EncodeToString(data), tail)
	}
	return "0x" + hex.EncodeToString(data)
}

// parseExtendedHexString parses a hex in the extended format (i.e. where "~" is
// used to indicated masked trailing bits).
func parseExtendedHexString(s string) ([]byte, uint) {
	var (
		splits    = strings.Split(s, "~")
		data, err = hex.DecodeString(splits[0])
		nbits     = uint(len(data) * 8)
	)
	//
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	} else if len(splits) > 2 {
		fmt.Fprintln(os.Stderr, "malformed extended hex string: ", s)
		os.Exit(2)
	} else if len(splits) != 1 {
		var mask uint64
		// parse mask
		if mask, err = strconv.ParseUint(splits[1], 10, 3); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(2)
		} else if len(data) == 0 {
			fmt.Fprintln(os.Stderr, "malformed extended hex string: ", s)
			os.Exit(2)
		} else if mask > 7 || mask == 0 {
			fmt.Fprintln(os.Stderr, "malformed extended hex string (invalid mask): ", s)
			os.Exit(2)
		} else if rest := data[len(data)-1] << mask; rest != 0 {
			fmt.Fprintln(os.Stderr, "malformed extended hex string (non-zero masked bits): ", s)
			os.Exit(2)
		}
		//
		nbits = nbits - 8 + uint(mask)
	}
	//
	return data, nbits
}

// encodeBytes packs one value per data line into a tightly-packed big-endian
// bit stream, cycling through the line widths; a mirror of the VM's EncodeBits
// and the inverse of decodeBytes.
func encodeBytes(values []uint64, widths []uint) ([]byte, uint) {
	var totalBits uint
	for i := range values {
		totalBits += widths[i%len(widths)]
	}
	out := make([]byte, (totalBits+7)/8)
	var off uint
	for i, v := range values {
		w := widths[i%len(widths)]
		for j := uint(0); j < w; j++ {
			if (v>>(w-1-j))&1 != 0 {
				out[(off+j)/8] |= 1 << (7 - (off+j)%8)
			}
		}
		off += w
	}
	return out, totalBits
}
`

// genericDecodeBytes is the layout-agnostic input decoder, emitted only when
// some input's data layout is not one of the specialized fast paths.  It
// preallocates the output (the cell count is known from the byte length and the
// round width), so it does not grow-and-copy while decoding.
const genericDecodeBytes = `// decodeBytes unpacks a tightly-packed big-endian bit stream into one value per
// data line, cycling through the line widths; a mirror of the VM's DecodeBits
// and the inverse of encodeBytes.
func decodeBytes(data []byte, total uint, widths []uint) []uint64 {
	var round uint
	for _, w := range widths {
		round += w
	}
	if round == 0 {
		return nil
	}
	if total%round != 0 {
		fmt.Fprintln(os.Stderr, "decode: partial row")
		os.Exit(2)
	}
	out := make([]uint64, 0, int(total/round)*len(widths))
	for off := uint(0); off < total; {
		for _, w := range widths {
			var v uint64
			for i := uint(0); i < w; i++ {
				bit := (data[(off+i)/8] >> (7 - (off+i)%8)) & 1
				v = v<<1 | uint64(bit)
			}
			out = append(out, v)
			off += w
		}
	}
	return out
}
`

// decodeU8Helper decodes a byte blob whose data layout is a single u8 line: one
// cell per input byte.  This is the common case (byte-oriented input), and the
// direct byte widen avoids the generic decoder's per-bit inner loop.
const decodeU8Helper = `// decodeU8 unpacks a single-u8-line byte blob into one cell per input byte.
func decodeU8(data []byte, nbits uint) []uint64 {
	if nbits%8 != 0 {
		fmt.Fprintln(os.Stderr, "decode: partial row")
		os.Exit(2)
	}
	out := make([]uint64, len(data))
	for i, b := range data {
		out[i] = uint64(b)
	}
	return out
}
`

// isSingleU8 reports whether a memory's data layout is a single u8 line, so its
// byte encoding is exactly one cell per input byte (the decodeU8 fast path).
func isSingleU8(widths []uint) bool {
	return len(widths) == 1 && widths[0] == 8
}

func (g *generator) emitConversionHelpers(c *code) {
	var anyU8, anyGeneric bool
	//
	c.commentf("===============================================================================")
	c.commentf("Encoding / Decoding Helpers")
	c.commentf("===============================================================================")
	c.line("")
	c.raw(conversionHelpers)
	c.line("")
	//
	for _, m := range g.inputs {
		if isSingleU8(m.dataWidths) {
			anyU8 = true
		} else {
			anyGeneric = true
		}
	}

	if anyU8 {
		c.raw(decodeU8Helper)
		c.line("")
	}

	if anyGeneric {
		c.raw(genericDecodeBytes)
		c.line("")
	}
}

// emitMainHarness writes the package-main entry point.  It reads the inputs as
// JSON (one hex/decimal string per input memory, matching `zkc exec`), decodes
// each into its data-line cells, runs the program, and writes the outputs as
// JSON hex strings in the same encoding.  Exit 1 signals an execution error (a
// rejected trace — the reference error path); exit 2 signals an I/O problem.
func (g *generator) emitMainHarness(c *code) error {
	c.line("func main() {")
	c.commentf("handler for failures / early termination")
	c.line("defer failureHandler()")
	// Flush buffered printf output on the way out.
	if g.usesHelper(helperDbgWriter) {
		c.line("defer dbgw.Flush()")
	}
	//
	c.commentf("read input memories")
	c.line("readInputs()")
	//
	if g.config.CheckPointing.HasValue() {
		if err := g.emitBootCheckpoint(c); err != nil {
			return err
		}
	}
	//
	c.commentf("execute zkc main function")
	c.line("zkc_main()")
	c.commentf("write output memories")
	c.line("writeOutputs()")
	c.line("}")
	//
	g.emitFailureHandler(c)
	g.emitInputReader(c)
	g.emitOutputWriter(c)
	//
	if g.config.CheckPointing.HasValue() {
		return g.emitCheckpointWriter(c)
	}
	//
	return nil
}

// emitFailureHandler emits the deferred handler which turns a ZkC failure into
// exit code 1 (the reference error path) and a DONE into normal, successful
// termination.  Since a DONE unwinds via panic from arbitrarily deep in the
// call stack, output collection for that path happens here rather than in
// main.  recover() must be called directly by the deferred function (and not,
// for example, be evaluated as an argument at the defer statement) for the
// panic to be caught.
func (g *generator) emitFailureHandler(c *code) {
	c.line("")
	c.line("func failureHandler() {")
	c.line("switch r := recover(); f := r.(type) {")
	c.line("case nil:")
	c.line("case failure:")
	c.line(`fmt.Fprintln(os.Stderr, "exec error:", f)`)
	c.line("os.Exit(1)")
	//
	if g.usesHelper(helperDone) {
		c.line("case doneSignal:")
		c.line("writeOutputs()")
	}
	//
	c.line("default:")
	c.line("panic(r)")
	c.line("}")
	c.line("}")
}

func (g *generator) emitInputReader(c *code) {
	c.line("")
	c.line("func readInputs() {")
	c.line("var raw map[string]string")
	c.commentf("read JSON from stdin")
	c.line("if err := json.NewDecoder(bufio.NewReader(os.Stdin)).Decode(&raw); err != nil {")
	c.line(`fmt.Fprintln(os.Stderr, "decode:", err)`)
	c.line("os.Exit(2)")
	c.line("}")

	for _, m := range g.inputs {
		c.line("{")
		c.linef("rawInput, ok := raw[%q]", m.name)
		c.line("if !ok {")
		c.linef(`fmt.Fprintf(os.Stderr, "decode: missing input %%q\n", %q)`, m.name)
		c.line("os.Exit(2)")
		c.line("}")
		c.line("data, nbits := parseInput(rawInput)")
		//
		if isSingleU8(m.dataWidths) {
			c.linef("%s = decodeU8(data, nbits)", m.varName)
		} else {
			c.linef("%s = decodeBytes(data, nbits, %s)", m.varName, widthsLiteral(m.dataWidths))
		}

		c.linef("delete(raw, %q)", m.name)
		c.line("}")
	}

	c.line("for name := range raw {")
	c.line(`fmt.Fprintf(os.Stderr, "decode: unknown input %q\n", name)`)
	c.line("os.Exit(2)")
	c.line("}")
	c.line("}")
}

func (g *generator) emitOutputWriter(c *code) {
	c.line("")
	c.line("func writeOutputs() {")
	//
	c.line("enc := map[string]string{}")
	// write outputs
	for _, m := range g.outputs {
		c.linef(`enc[%q] = hexString(encodeBytes(%s, %s))`, m.name, m.varName, widthsLiteral(m.dataWidths))
	}
	//
	c.commentf("write JSON to stdout")
	c.line("if err := json.NewEncoder(os.Stdout).Encode(enc); err != nil {")
	c.line(`fmt.Fprintln(os.Stderr, "encode:", err)`)
	c.line("os.Exit(2)")
	c.line("}")
	c.line("}")
}

// emitCheckpointWriter emits writeCheckpoint(fid, args), which serialises the
// current machine state in the binary format decoded by checkpoint.Unmarshal
// (see checkpoint/encoding.go) and writes it to stdout as a single JSON line of
// the form {"$":"0x…"}.  The caller supplies the identifier of the function
// being entered along with its (non-stamp) arguments, already packed as per
// interpreter.Pack (see emitCheckpointArgs).  As with interpreter.Checkpoint,
// every non-static memory is included in module order (Restore relies on
// this), with page contents packed likewise (a fixed number of bytes per cell)
// and the memory clock as tracked by the generated program.
func (g *generator) emitCheckpointWriter(c *code) error {
	c.line("")
	c.line("func writeCheckpoint(fid uint16, args []byte) {")
	c.line("var buf []byte")
	c.commentf("function")
	c.line("buf = binary.BigEndian.AppendUint16(buf, fid)")
	c.commentf("arguments")
	c.line("buf = binary.BigEndian.AppendUint32(buf, uint32(len(args)))")
	c.line("buf = append(buf, args...)")
	//
	g.emitCheckpointWriterMemories(c)
	//
	c.commentf("write JSON to stdout")
	c.line(`if _, err := fmt.Fprintf(os.Stdout, "{\"$\":\"0x%s\"}\n", hex.EncodeToString(buf)); err != nil {`)
	c.line(`fmt.Fprintln(os.Stderr, "checkpoint:", err)`)
	c.line("os.Exit(2)")
	c.line("}")
	c.line("}")
	//
	g.emitCheckpointWriterHelpers(c)
	//
	return nil
}

// emitCheckpointArgs emits a local "args" holding the given (non-stamp input)
// limbs of fn packed as checkpoint argument bytes, mirroring interpreter.Pack:
// each register big endian in cellByteWidth bytes.
func (g *generator) emitCheckpointArgs(c *code, fn *descFunction, args []limb) {
	var native = lword.ByteWidth(g.program.Field().BandWidth)
	//
	c.line("var args []byte")
	//
	for _, l := range args {
		var (
			r = fn.Register(l.id)
			n = cellByteWidth(r, native)
		)
		//
		c.commentf("argument %q", r.Name())
		// A wide (lo/hi pair) register packs big endian as the hi limb in the
		// leading n-8 bytes followed by the lo limb in the trailing 8.
		if l.width > 64 {
			c.linef("args = encodeCell(args, %s, %d)", l.hiName(), n-8)
			c.linef("args = encodeCell(args, %s, 8)", l.lo())
		} else {
			c.linef("args = encodeCell(args, %s, %d)", l.lo(), n)
		}
	}
}

// emitBootCheckpoint emits the initial checkpoint written by main() before
// execution begins, mirroring BootAndCheckpoint: it records the boot frame
// (main) whose (non-stamp) inputs are all zero.  The trace builder relies on
// this being the first checkpoint, as the first shard is traced from boot.
func (g *generator) emitBootCheckpoint(c *code) error {
	var (
		mid, _ = g.program.HasModule("main")
		fn     = g.funcByID[uint(mid)]
		native = lword.ByteWidth(g.program.Field().BandWidth)
		nbytes uint
	)
	//
	args, err := g.checkpointArgs(fn)
	if err != nil {
		return err
	}
	//
	for _, l := range args {
		nbytes += cellByteWidth(fn.Register(l.id), native)
	}
	//
	c.commentf("boot checkpoint (inputs of main are zero)")
	//
	if nbytes == 0 {
		c.linef("writeCheckpoint(%d, nil)", mid)
	} else {
		c.linef("writeCheckpoint(%d, make([]byte, %d))", mid, nbytes)
	}
	//
	return nil
}

func (g *generator) emitCheckpointWriterMemories(c *code) {
	var (
		// Non-static memories, in module order.
		memories []memInfo
	)
	// Extract memories
	for id := range g.program.Modules() {
		if info, ok := g.memByID[uint(id)]; ok && info.role != sromStatic {
			memories = append(memories, info)
		}
	}
	//
	c.commentf("%d memories", len(memories))
	c.linef("buf = binary.BigEndian.AppendUint32(buf, %d)", len(memories))
	//
	for _, m := range memories {
		widths := widthsLiteral(g.cellByteWidths(m))
		//
		c.commentf("memory %q", m.name)
		//
		switch m.role {
		case pagedScratch:
			rows := pagedPageSize / uint64(len(m.geom.DataRegisters()))
			c.linef("buf = encodePaged(buf, %d, %s, %s_stamp, %d, %s)", m.id, m.varName, m.varName, rows, widths)
		case ramScratch:
			c.linef("buf = encodeMemory(buf, %d, %s, %s_stamp, %s)", m.id, m.varName, m.varName, widths)
		default:
			c.linef("buf = encodeMemory(buf, %d, %s, 0, %s)", m.id, m.varName, widths)
		}
	}
}

func (g *generator) emitCheckpointWriterHelpers(c *code) {
	c.commentf("===============================================================================")
	c.commentf("Checkpoint Encoders")
	c.commentf("===============================================================================")
	// encodeCell appends v big endian in exactly n bytes.
	c.line("")
	c.line("func encodeCell(buf []byte, v uint64, n uint) []byte {")
	c.line("for j := range n {")
	c.line("buf = append(buf, byte(v>>(8*(n-1-j))))")
	c.line("}")
	c.line("return buf")
	c.line("}")
	c.line("")
	// encodePage appends one checkpoint page: u64 address, u32 len(data), data
	// (cells packed big endian using widths[i % len(widths)] bytes for cell i)
	// and u32 len(stamps) == 0.
	c.line("func encodePage(buf []byte, address uint64, cells []uint64, widths []uint) []byte {")
	c.line("var n uint")
	c.line("for i := range cells {")
	c.line("n += widths[i%len(widths)]")
	c.line("}")
	c.line("buf = binary.BigEndian.AppendUint64(buf, address)")
	c.line("buf = binary.BigEndian.AppendUint32(buf, uint32(n))")
	c.line("for i, v := range cells {")
	c.line("buf = encodeCell(buf, v, widths[i%len(widths)])")
	c.line("}")
	c.line("return binary.BigEndian.AppendUint32(buf, 0)")
	c.line("}")
	c.line("")
	// encodeMemory appends one memory header (u16 module, u64 clock == 0) followed
	// by a single page at address zero holding all cells.
	c.line("func encodeMemory(buf []byte, mid uint16, cells []uint64, stamp uint64, widths []uint) []byte {")
	c.commentf("module identifier")
	c.line("buf = binary.BigEndian.AppendUint16(buf, mid)")
	c.commentf("timestamp")
	c.line("buf = binary.BigEndian.AppendUint64(buf, stamp)")
	c.commentf("page count")
	c.line("buf = binary.BigEndian.AppendUint32(buf, 1)")
	c.line("return encodePage(buf, 0, cells, widths)")
	c.line("}")
	c.line("")
	//
	if len(g.pageds) > 0 {
		// encodePaged is as encodeMemory, but with one page per allocated page of a
		// paged memory, addressed by its first row (rows is rows per page).
		c.line("func encodePaged(buf []byte, mid uint16, pages [][]uint64, stamp, rows uint64, widths []uint) []byte {")
		c.line("var n uint32")
		c.line("for _, p := range pages {")
		c.line("if p != nil {")
		c.line("n++")
		c.line("}")
		c.line("}")
		c.commentf("module identifier")
		c.line("buf = binary.BigEndian.AppendUint16(buf, mid)")
		c.commentf("timestamp")
		c.line("buf = binary.BigEndian.AppendUint64(buf, stamp)")
		c.commentf("page count")
		c.line("buf = binary.BigEndian.AppendUint32(buf, n)")
		c.line("for i, p := range pages {")
		c.line("if p != nil {")
		c.line("buf = encodePage(buf, uint64(i)*rows, p, widths)")
		c.line("}")
		c.line("}")
		c.line("return buf")
		c.line("}")
		c.line("")
	}
}

// cellByteWidths returns, for each data lane of the given memory, the number
// of bytes used to encode one cell of that lane in a checkpoint page.
func (g *generator) cellByteWidths(info memInfo) []uint {
	var (
		regs   = info.geom.DataRegisters()
		native = lword.ByteWidth(g.program.Field().BandWidth)
		widths = make([]uint, len(regs))
	)
	//
	for i, r := range regs {
		widths[i] = cellByteWidth(r, native)
	}
	//
	return widths
}

// cellByteWidth returns the number of bytes used to encode one cell of the
// given register in a checkpoint.  This mirrors interpreter.Pack: the
// register's bytewidth or, for native registers, that of the field's bandwidth
// (given here as native).
func cellByteWidth(r descriptor.Register[word.Uint], native uint) uint {
	if r.IsNative() {
		return native
	}
	//
	return lword.ByteWidth(r.Bitwidth().Unwrap())
}

// checkpointArgs returns the limbs of the given (shard) function's non-stamp
// input registers, in register order.  These are the arguments recorded in a
// checkpoint (mirroring Function.InputsExStamps), and hence the parameters of
// the generated writeCheckpoint.
func (g *generator) checkpointArgs(fn *descFunction) ([]limb, error) {
	var limbs []limb
	//
	for i := range fn.NumInputs() {
		if fn.Register(regId(i)).IsStamp() {
			continue
		}
		//
		l, err := g.limbOf(fn, regId(i))
		//
		if err != nil {
			return nil, err
		}
		//
		limbs = append(limbs, l)
	}
	//
	return limbs, nil
}

// widthsLiteral renders a Go []uint literal of data-line bit-widths.
func widthsLiteral(widths []uint) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = fmt.Sprintf("%d", w)
	}

	return "[]uint{" + strings.Join(parts, ", ") + "}"
}

// ===========================================================================
// Function emission
// ===========================================================================

// goFuncName returns the Go function name for a ZkC function.
func goFuncName(fn *descFunction) string { return "zkc_" + sanitize(fn.Name()) }

// emitFunction emits a complete Go function: signature, register locals, body.
// Input registers become parameters (named rN so the operand helpers address
// them unchanged) and output registers become plain results.  The boot frame
// ('main') takes no parameters — its inputs are zero, matching the boot frame's
// zero-allocated activation record — and discards its outputs.
func (g *generator) emitFunction(c *code, id uint, fn *descFunction, cfg Config) error {
	//
	if cfg.CheckPointing.HasValue() {
		var cp = cfg.CheckPointing.Unwrap()
		//
		if cp.ShardFunction == fn.Name() {
			c.linef("// checkpointing clock for function \"%s\"", fn.Name())
			c.linef("var clk_%s uint64 = %d", fn.Name(), cfg.CheckPointing.Unwrap().ShardSteps)
		}
	}
	//
	if err := g.emitFunctionSignature(c, fn); err != nil {
		return err
	} else if err = g.emitFunctionBody(c, id, fn, cfg); err != nil {
		return err
	}
	//
	c.line("}")
	c.line("")
	//
	return nil
}

func (g *generator) emitFunctionSignature(c *code, fn *descFunction) error {
	var (
		isBoot   = fn.Name() == "main"
		nInputs  = fn.NumInputs()
		nOutputs = fn.NumOutputs()
		outNames []string
		retType  string
		paramFmt []string
	)
	// Parameters
	for i := range nInputs {
		l, err := g.limbOf(fn, regId(i))
		if err != nil {
			return err
		}

		if l.width > 64 {
			paramFmt = append(paramFmt, fmt.Sprintf("%s, %s uint64", l.lo(), l.hiName()))
		} else {
			paramFmt = append(paramFmt, fmt.Sprintf("%s uint64", l.lo()))
		}
	}
	// Returns
	for i := range nOutputs {
		l, err := g.limbOf(fn, regId(nInputs+i))
		if err != nil {
			return err
		}

		outNames = append(outNames, l.lo())
		if l.width > 64 {
			outNames = append(outNames, l.hiName())
		}
	}
	//
	switch len(outNames) {
	case 0:
	case 1:
		retType = " uint64"
	default:
		retType = " (" + strings.TrimSuffix(strings.Repeat("uint64, ", len(outNames)), ", ") + ")"
	}
	//
	c.commentf("%s corresponds to ZkC function %q.", goFuncName(fn), fn.Name())
	c.linef("func %s(%s)%s {", goFuncName(fn), strings.Join(paramFmt, ", "), retType)
	//
	g.cur = fnView{isBoot: isBoot, outNames: outNames}
	//
	return nil
}

func (g *generator) emitFunctionBody(c *code, id uint, fn *descFunction, cfg Config) error {
	var (
		isBoot = fn.Name() == "main"
		regs   = fn.Registers()
	)
	// Emit the body to a fixpoint: each pass replays the emitters (the
	// analysis transfer function) over the whole body; label states stabilise
	// within a few passes thanks to widening (see intervals).  The final
	// stable pass IS the emitted body.  Helper flags are reset per pass so
	// only the final pass decides them.
	var (
		body                             code
		savedBits, savedMP, savedHelpers = g.usesBits, g.usesModP, cloneHelpers(g.helpers)
	)

	g.iv = newIntervals(fn, isBoot, g.config.NoIntervals)

	for pass := 0; ; pass++ {
		body = code{}
		g.usesBits, g.usesModP = savedBits, savedMP
		g.helpers = cloneHelpers(savedHelpers)
		g.iv.beginPass()

		if err := g.emitFunctionVectors(&body, id, fn); err != nil {
			return err
		}

		if g.iv.stable() {
			break
		}
		// Safety valve: an unexpected non-terminating fixpoint falls back to
		// width-derived bounds (the next pass is then trivially stable).
		if pass >= 8 {
			g.iv.disabled = true
		}
	}
	// The self-tail-call back edge lands BEFORE the local declarations, so each
	// iteration re-executes them and starts from a zeroed frame, exactly like
	// the interpreter's frame reuse.
	if g.cur.tailLoop {
		c.line("entry:")
	}
	//
	used, read := usedRegisters(body.String())
	// Variable declarations
	for i := fn.NumInputs(); i < uint(len(regs)); i++ {
		l, err := g.limbOf(fn, regId(i))
		if err != nil {
			return err
		}

		tokens := []string{l.lo()}
		if l.width > 64 {
			tokens = append(tokens, l.hiName())
		}

		for _, tok := range tokens {
			if !used[tok] {
				continue
			}

			c.linef("var %s uint64 // %s", tok, regs[i].Name())
			// Go rejects a variable that is only ever assigned; a write-only
			// register (e.g. a dead destructure limb) needs a blank use.
			if !read[tok] {
				c.linef("_ = %s", tok)
			}
		}
	}
	//
	if cfg.CheckPointing.HasValue() {
		if strategy := cfg.CheckPointing.Unwrap(); strategy.ShardFunction == fn.Name() {
			// The checkpoint records this function's (non-stamp) arguments,
			// packed from the parameters at the point of capture.
			args, err := g.checkpointArgs(fn)
			//
			if err != nil {
				return err
			}
			//
			c.commentf("decrement clock")
			c.linef("clk_%s = clk_%s - 1", strategy.ShardFunction, strategy.ShardFunction)
			c.linef("if clk_%s == 0 {", strategy.ShardFunction)
			c.commentf("reset clock")
			c.linef("clk_%s = %d", strategy.ShardFunction, strategy.ShardSteps)
			c.commentf("capture checkpoint")
			g.emitCheckpointArgs(c, fn, args)
			c.linef("writeCheckpoint(%d, args)", id)
			c.linef("}")
		}
	}

	//
	c.raw(body.String())
	//
	return nil
}

// usedRegisters scans generated code for register-limb tokens (rN, rN_0,
// rN_1), so emitFunction only declares the locals the body actually uses —
// and, separately, which of them are ever READ (an emitted assignment always
// has the `rX[, rY…] = rhs` shape, so tokens left of a leading `=` are writes;
// everything else is a read).  Comments are excluded: they quote ZkC register
// names, which may themselves look like rN.
func usedRegisters(body string) (used, read map[string]bool) {
	used, read = map[string]bool{}, map[string]bool{}

	collect := func(s string, into map[string]bool) {
		for _, tok := range regRefPattern.FindAllString(s, -1) {
			into[tok] = true
		}
	}

	for line := range strings.SplitSeq(body, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}

		if m := assignPattern.FindStringSubmatch(line); m != nil {
			collect(m[1], used) // assignment targets: used, not read
			collect(m[2], read) // right-hand side: reads
			collect(m[2], used)
		} else {
			collect(line, read)
			collect(line, used)
		}
	}

	return used, read
}

var (
	regRefPattern = regexp.MustCompile(`\br\d+(?:_[01])?\b`)
	// assignPattern matches the emitted assignment shape `rX[, rY…] = rhs`
	// (plain `=` only — the emitters never use register tokens with `:=` or
	// compound ops).
	assignPattern = regexp.MustCompile(`^\s*(r\d+(?:_[01])?(?:, r\d+(?:_[01])?)*) = (.*)$`)
)

// returnOk renders the `return` performed by a RETURN instruction: the boot
// frame discards its outputs, whereas a callee returns its output registers
// (limb-expanded).
func (g *generator) returnOk() string {
	if len(g.cur.outNames) == 0 {
		return "return"
	}

	return "return " + strings.Join(g.cur.outNames, ", ")
}

// ===========================================================================
// Instruction emission
// ===========================================================================

// emitFunctionVectors walks the (vectorised) code, emitting one block per micro-
// instruction in program order, preceded by a label wherever a skip or jump
// targets that position.  A final (unreachable for well-formed code) panic
// satisfies Go's terminating-statement requirement and models falling off the
// end of the function.
func (g *generator) emitFunctionVectors(c *code, id uint, fn *descFunction) error {
	mapping := g.program.EnvironmentOf(uint16(id))
	code := fn.Vectors()
	labels := collectLabels(code)

	for vi, vec := range code {
		n := uint(len(vec.Bytecodes))
		for ci, insn := range vec.Bytecodes {
			at := pos{uint(vi), uint(ci)}
			if labels[at] {
				c.linef("%s:", labelName(at))
				g.iv.atLabel(at)
			}

			c.commentf("[%d.%d] %s", vi, ci, insn.String(mapping))

			if err := g.emitInstruction(c, fn, insn, uint(vi), uint(ci), n); err != nil {
				return err
			}
		}
	}
	// Fall-off-the-end target: only a malformed (non-terminating) program reaches
	// it, but it also guarantees the Go function ends in a terminating statement.
	// When the last instruction already transfers control and nothing jumps past
	// the end, the panic would be unreachable (go vet objects) — omit it.
	end := pos{uint(len(code)), 0}
	if labels[end] {
		c.linef("%s:", labelName(end))
	}

	if labels[end] || !endsTerminated(code, id) {
		c.line(`panic(failure("machine fell off end of function"))`)
	}

	return nil
}

// endsTerminated reports whether the final instruction of the code (if any)
// unconditionally transfers control, i.e. execution cannot fall off the end.
// A self tail call counts: it is emitted as an unconditional goto.
func endsTerminated(code BytecodeVector, id uint) bool {
	if len(code) == 0 {
		return false
	}

	last := code[len(code)-1].Bytecodes

	if len(last) == 0 {
		return false
	}

	switch b := last[len(last)-1].(type) {
	case *bytecode.Skip[word.Uint], *bytecode.Jmp[word.Uint], *bytecode.Ret[word.Uint], *bytecode.Fail[word.Uint]:
		return true
	case *bytecode.Call[word.Uint]:
		return b.Never && uint(b.Target) == id
	default:
		return false
	}
}

func (g *generator) emitInstruction(c *code, fn *descFunction, insn bytecode.Bytecode[word.Uint],
	vi, ci, vecLen uint) error {
	switch x := insn.(type) {
	case *bytecode.Arith[word.Uint]:
		return g.emitArith(c, fn, x)
	case *bytecode.Bitwise[word.Uint]:
		return g.emitBitwise(c, fn, x)
	case *bytecode.Cat[word.Uint]:
		return g.emitConcat(c, fn, x)
	case *bytecode.CheckCast[word.Uint]:
		return g.emitCheckCast(c, fn, x)
	case *bytecode.DivMod[word.Uint]:
		return g.emitDivMod(c, fn, x)
	case *bytecode.FieldArith[word.Uint]:
		return g.emitFieldOp(c, fn, x)
	case *bytecode.UintToField[word.Uint]:
		return g.emitUintToField(c, fn, x)
	case *bytecode.FieldToUint[word.Uint]:
		return g.emitFieldToUint(c, fn, x)
	case *bytecode.Intrinsic[word.Uint]:
		return g.emitIntrinsic(c, fn, x)
	case *bytecode.Debug[word.Uint]:
		// DEBUG (printf) has no effect on program outputs; it writes to stderr.
		return g.emitDebug(c, fn, x)
	case *bytecode.ReadWrite[word.Uint]:
		// The unconditional flag on a call (range checks) and the checkpoint
		// flag affect only constraint lowering, which gogen does not perform;
		// likewise memory reads/writes execute identically whatever their kind.
		if x.Write {
			return g.emitMemWrite(c, fn, x)
		}

		return g.emitMemRead(c, fn, x)
	case *bytecode.Call[word.Uint]:
		return g.emitCall(c, fn, x)
	case *bytecode.Skip[word.Uint]:
		target := skipTarget(vi, ci, uint(x.Skip), vecLen)
		c.linef("goto %s", labelName(target))
		g.iv.edgeTo(target)
		g.iv.endOfFlow()

		return nil
	case *bytecode.SkipIf[word.Uint]:
		cond, err := g.condExpr(fn, x)
		if err != nil {
			return err
		}

		target := skipTarget(vi, ci, uint(x.Skip), vecLen)

		c.linef("if %s {", cond)
		c.linef("goto %s", labelName(target))
		c.line("}")
		g.iv.edgeTo(target)

		return nil
	case *bytecode.Switch[word.Uint]:
		return g.emitMultiwaySkip(c, fn, x, vi, ci, vecLen)
	case *bytecode.Dispatch[word.Uint]:
		return g.emitDispatch(c, fn, x, vi, ci, vecLen)
	case *bytecode.Jmp[word.Uint]:
		target := pos{uint(x.Target), 0}
		c.linef("goto %s", labelName(target))
		g.iv.edgeTo(target)
		g.iv.endOfFlow()

		return nil
	case *bytecode.Ret[word.Uint]:
		if x.Done {
			g.useHelper(helperDone)
			c.line("done()")
		} else {
			c.line(g.returnOk())
		}

		g.iv.endOfFlow()

		return nil
	case *bytecode.Fail[word.Uint]:
		if err := g.emitFail(c, fn, x); err != nil {
			return err
		}

		g.iv.endOfFlow()

		return nil
	default:
		return fmt.Errorf("gogen: unsupported bytecode %T", insn)
	}
}

// ===========================================================================
// Misc helpers
// ===========================================================================

// reg returns the Go local name for a register id.
func reg(id regId) string { return fmt.Sprintf("r%d", id) }

// widthOf returns the width of a register as represented in generated code:
// its declared width, or 64 for a native (field-element) register, which is a
// single uint64 local — a field value fits uint64 but need not be reduced mod
// P (see regWidth, which additionally guards the modulus precondition).
func widthOf(r descriptor.Register[word.Uint]) uint {
	if r.IsNative() {
		return 64
	}

	return r.Bitwidth().Unwrap()
}

func (g *generator) regWidth(fn *descFunction, id regId) (uint, error) {
	r := fn.Register(id)
	if r.IsNative() {
		// A native (field-element) register is a single uint64 local when the
		// field modulus fits 64 bits: a field value fits uint64, and the mod-P
		// emitters (emit_field.go) reduce operands as they combine them — so a
		// value at rest need not be reduced (a stored constant may exceed P),
		// exactly mirroring the interpreter, which reduces only inside field
		// ops.  A wider modulus (e.g. BLS12-377) needs a multi-limb
		// representation and stays out of scope.
		if g.modulus.BitLen() > 64 {
			return 0, fmt.Errorf("gogen: native register r%d unsupported for modulus wider than 64 bits", id)
		}

		return 64, nil
	}
	// Registers up to 64 bits are single uint64 locals; up to 128 bits they
	// are rN_0/rN_1 limb pairs.
	if w := r.Bitwidth().Unwrap(); w <= 128 {
		return w, nil
	}

	return 0, fmt.Errorf("gogen: register %q wider than 128 bits (u%d) unsupported",
		r.Name(), r.Bitwidth().Unwrap())
}

// uintConst converts a word.Uint constant into a uint64, erroring on wider
// constants (which only occur alongside wide registers, equally unsupported).
func uintConst(w word.Uint) (uint64, error) {
	if !w.FitsWithin(64) {
		return 0, fmt.Errorf("gogen: constant 0x%s wider than 64 bits unsupported", w.Text(16))
	}

	return w.Uint64(), nil
}

// constOperand renders an instruction's immediate as an exact operand,
// supporting up to 128 bits via the lo/hi pair representation.  The lowering
// passes introduce wide masks (e.g. the 0xff…ff produced by bitwise lowering)
// which then flow through the arithmetic emitters like any other wide value.
func constOperand(w word.Uint) (operand, error) {
	if !w.FitsWithin(128) {
		return operand{}, fmt.Errorf("gogen: constant 0x%s wider than 128 bits unsupported", w.Text(16))
	}

	return exactWide(w.BigInt()), nil
}

// hasNativeDataLine reports whether a memory has a field-element (native) data
// line, which has no fixed bit-width and so cannot cross the JSON byte boundary
// the main harness uses.
func hasNativeDataLine(info memInfo) bool {
	for _, r := range info.geom.DataRegisters() {
		if r.IsNative() {
			return true
		}
	}

	return false
}

func (g *generator) classifyMemory(m *descriptor.Memory[word.Uint]) (memInfo, error) {
	info := memInfo{name: m.Name(), varName: g.uniqueName("zkc_" + sanitize(m.Name())), geom: *m}
	// All memory traffic moves through uint64 cells.
	for _, r := range append(info.geom.AddressRegisters(), info.geom.DataRegisters()...) {
		if !r.IsNative() && r.Bitwidth().Unwrap() > 64 {
			return info, fmt.Errorf("gogen: memory %q register %q wider than 64 bits unsupported", m.Name(), r.Name())
		}
	}

	// Record data-line widths for the byte encoding the main harness shares
	// with `zkc exec`.  Native (field-element) lines have no fixed width, so
	// memories using them are scratch-only — the main harness rejects them as
	// input/output (see Generate).
	if !hasNativeDataLine(info) {
		for _, r := range info.geom.DataRegisters() {
			info.dataWidths = append(info.dataWidths, r.Bitwidth().Unwrap())
		}
	}

	switch {
	case m.IsStatic():
		// Static read-only: bake the fixed contents into the generated program.
		info.role = sromStatic

		contents, err := toU64s(m.StaticContents())
		if err != nil {
			return info, fmt.Errorf("gogen: memory %q: %w", m.Name(), err)
		}

		info.contents = contents
		info.elemType = staticElemType(contents)
	case m.IsPaged():
		info.role = pagedScratch
	case m.IsReadOnly():
		info.role = romInput
	case m.IsWriteOnly():
		info.role = womOutput
	case m.IsReadWrite():
		info.role = ramScratch
	default:
		return info, fmt.Errorf("gogen: unsupported memory %q (%T)", m.Name(), m)
	}

	return info, nil
}

// toU64s converts SROM contents into plain uint64s for baking.
func toU64s(words []word.Uint) ([]uint64, error) {
	out := make([]uint64, len(words))

	for i, w := range words {
		v, err := uintConst(w)
		if err != nil {
			return nil, err
		}

		out[i] = v
	}

	return out, nil
}

// sromLiteral renders a compact, fixed-size static-memory literal.  A fixed
// array ([...]) lets the Go compiler drop bounds checks when the index is
// provably in range, and the narrowest Go type covering every baked value keeps
// large ROMs small (reads explicitly widen back to the VM's uint64 cell type).
func sromLiteral(info memInfo) string {
	elem := info.elemType
	if len(info.contents) == 0 {
		return "[...]" + elem + "{}"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[...]%s{\n", elem)

	const valuesPerLine = 8
	for i, v := range info.contents {
		if i%valuesPerLine == 0 {
			b.WriteString("\t")
		}

		fmt.Fprintf(&b, "0x%x, ", v)

		if i%valuesPerLine == valuesPerLine-1 || i == len(info.contents)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("}")

	return b.String()
}

// staticElemType is the narrowest unsigned Go type that holds every baked value.
// Sizing from the actual contents (rather than the declared data-line width)
// keeps the literal correct by construction and as small as the values allow.
func staticElemType(contents []uint64) string {
	var maxV uint64
	for _, v := range contents {
		maxV = max(maxV, v)
	}

	switch {
	case maxV <= 0xff:
		return "uint8"
	case maxV <= 0xffff:
		return "uint16"
	case maxV <= 0xffffffff:
		return "uint32"
	default:
		return "uint64"
	}
}

// uniqueName reserves a sanitized identifier, suffixing on collision (distinct
// ZkC names may sanitize to the same Go identifier).
func (g *generator) uniqueName(name string) string {
	for i := 0; ; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d", name, i)
		}

		if !g.names[candidate] {
			g.names[candidate] = true
			return candidate
		}
	}
}

func sanitize(name string) string {
	var b strings.Builder

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}

	return b.String()
}

func commentText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// formatSource runs gofmt over generated source, surfacing a numbered listing on
// failure to make codegen bugs easy to locate.
func formatSource(src string) (string, error) {
	out, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("gogen: generated source is invalid Go: %w\n%s", err, numberLines(src))
	}

	return string(out), nil
}

func numberLines(src string) string {
	var b strings.Builder
	for i, line := range strings.Split(src, "\n") {
		fmt.Fprintf(&b, "%4d\t%s\n", i+1, line)
	}

	return b.String()
}
