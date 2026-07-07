// Copyright Consensys Software Inc.
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

package gogen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/bytecode"
)

// A debug/fail bytecode carries its formatted-print specification as literal
// chunks plus a separate list of source register vectors: a chunk consumes the
// next source vector exactly when it carries a format (mirroring the
// interpreter's formatChunks).

// emitDebug renders a printf (DEBUG) instruction as a sequence of writes to the
// buffered stderr writer dbgw.  Output is byte-for-byte compatible with the
// reference interpreter (formatChunks / formatArgument): literal text is
// written verbatim, integers go out in the requested base with digit-only width
// padding (no base prefix, lowercase hex/bin) and %c as a single raw byte.
//
// The per-chunk writes use specialised primitives (WriteString / strconv /
// big.Int) rather than fmt: a non-quiet run executes one printf per
// instruction, and fmt.Fprintf would re-parse the format string and box every
// argument on each call.  Output goes to stderr (not stdout) so it never
// corrupts the JSON the package-main harness writes to stdout.  DEBUG
// instructions are present only in non-quiet builds — the compiler drops printf
// under --quiet — so this is dead code there.
func (g *generator) emitDebug(c *code, fn *descFunction, x *bytecode.Debug) error {
	g.useHelper(helperDbgWriter)

	next := 0

	for _, ch := range x.Chunks {
		if ch.Text != "" {
			c.linef("dbgw.WriteString(%s)", strconv.Quote(ch.Text))
		}

		if !ch.Format.HasFormat() {
			continue
		}

		vec := x.Sources[next]
		next++

		if err := g.emitDebugArg(c, fn, ch.Format, vec.Registers()); err != nil {
			return err
		}
	}

	return nil
}

// emitDebugArg writes one formatted printf argument to dbgw.  The common case —
// a single register no wider than 64 bits — formats via dbgU; %c writes the low
// byte verbatim; wider or multi-register arguments fold their limbs into a
// *big.Int (matching formatArgument) and format via dbgB.
func (g *generator) emitDebugArg(c *code, fn *descFunction, format util.Format, regs []regId) error {
	// %c writes the low byte verbatim (type-checked to a single u8).
	if format.Code == util.FORMAT_CHR {
		op, err := g.operand(fn, regs[0])
		if err != nil {
			return err
		}

		c.linef("dbgw.WriteByte(byte(%s))", op.expr)

		return nil
	}

	pad := "' '"
	if format.ZeroPad {
		pad = "'0'"
	}

	if len(regs) == 1 {
		op, err := g.operand(fn, regs[0])
		if err != nil {
			return err
		}

		if op.wide() {
			g.useHelper(helperDbgB)
			g.useHelper(helperU128)

			c.linef("dbgB(u128(%s, %s), %d, %d, %s)", op.expr, op.hiOr0(), formatBase(format), format.Width, pad)

			return nil
		}

		g.useHelper(helperDbgU)

		c.linef("dbgU(%s, %d, %d, %s)", op.expr, formatBase(format), format.Width, pad)

		return nil
	}

	val, err := g.multiLimbBig(fn, regs)
	if err != nil {
		return err
	}

	g.useHelper(helperDbgB)

	c.linef("dbgB(%s, %d, %d, %s)", val, formatBase(format), format.Width, pad)

	return nil
}

// formatBase maps a numeric format to its strconv/big.Int base.  %c is handled
// by the caller and never reaches here.
func formatBase(f util.Format) int {
	switch f.Code {
	case util.FORMAT_HEX:
		return 16
	case util.FORMAT_BIN:
		return 2
	default: // FORMAT_DEC
		return 10
	}
}

// emitFail renders a FAIL instruction.  With no message chunks it panics with
// the bare "machine panic" the interpreter reports; otherwise it formats the
// message exactly as the interpreter does and panics with "machine panic:
// <msg>".  The Run entry point recovers the failure into its error result,
// which the harness relays on stderr.  FAIL is always compiled (quiet only
// strips printf), so the message path is live in both modes.
func (g *generator) emitFail(c *code, fn *descFunction, x *bytecode.Fail) error {
	if len(x.Chunks) == 0 {
		c.line(`panic(failure("machine panic"))`)
		return nil
	}

	format, plain, args, err := g.printfChunks(fn, x.Chunks, x.Sources)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		c.linef("panic(failure(%s))", strconv.Quote("machine panic: "+plain))
		return nil
	}

	// FAIL is a cold, terminating path (executed at most once), so it keeps the
	// simpler fmt.Sprintf formatting rather than the specialised writes used for
	// the hot DEBUG path.
	g.useHelper(helperFmtSprintf)

	c.linef("panic(failure(fmt.Sprintf(%s, %s)))", strconv.Quote("machine panic: "+format), strings.Join(args, ", "))

	return nil
}

// printfChunks assembles a printf/fail's chunks into (a) a Go format string
// with the interpreter's verbs, (b) the equivalent plain text for the
// no-argument case, and (c) the matching argument expressions.  Literal text
// has its '%' doubled so it survives Fprintf; the plain form keeps it verbatim.
func (g *generator) printfChunks(fn *descFunction, chunks []bytecode.FormattedChunk,
	sources []bytecode.RegisterVector) (format, plain string, args []string, err error) {
	//
	var (
		fb, pb strings.Builder
		next   = 0
	)

	for _, ch := range chunks {
		fb.WriteString(strings.ReplaceAll(ch.Text, "%", "%%"))
		pb.WriteString(ch.Text)

		if !ch.Format.HasFormat() {
			continue
		}

		vec := sources[next]
		next++

		verb, arg, e := g.printfArg(fn, ch.Format, vec.Registers())
		if e != nil {
			return "", "", nil, e
		}

		fb.WriteString(verb)

		args = append(args, arg)
	}

	return fb.String(), pb.String(), args, nil
}

// printfArg returns the Go verb and argument expression for one formatted
// chunk.  The common case — a single register no wider than 64 bits — passes
// the local directly; %c renders the low byte verbatim; wider or multi-register
// arguments fold their limbs (most-significant last, matching formatArgument)
// into a *big.Int, which fmt formats identically for %d/%x/%b.
func (g *generator) printfArg(fn *descFunction, format util.Format, regs []regId) (string, string, error) {
	if len(regs) == 1 {
		op, err := g.operand(fn, regs[0])
		if err != nil {
			return "", "", err
		}

		if format.Code == util.FORMAT_CHR {
			g.useHelper(helperChr)
			return "%s", fmt.Sprintf("chr(%s)", op.expr), nil
		}

		if op.wide() {
			g.useHelper(helperU128)
			return format.String(), fmt.Sprintf("u128(%s, %s)", op.expr, op.hiOr0()), nil
		}

		return format.String(), op.expr, nil
	}

	// Multi-register argument: concatenate the limbs into one big.Int.  %c is
	// type-checked to a single u8, so it never reaches here.
	if format.Code == util.FORMAT_CHR {
		return "", "", fmt.Errorf("gogen: %%c with a multi-register argument unsupported")
	}

	arg, err := g.multiLimbBig(fn, regs)
	if err != nil {
		return "", "", err
	}

	return format.String(), arg, nil
}

// multiLimbBig builds a catU64 call folding a multi-register argument's limbs
// (least-significant first, matching formatArgument) into one *big.Int.  Each
// limb must be a single ≤64-bit register.
func (g *generator) multiLimbBig(fn *descFunction, regs []regId) (string, error) {
	var vals, widths []string

	for _, id := range regs {
		op, err := g.operand(fn, id)
		if err != nil {
			return "", err
		}

		if op.wide() {
			return "", fmt.Errorf("gogen: printf argument with a >64-bit limb in a multi-register vector unsupported")
		}

		w, err := g.regWidth(fn, id)
		if err != nil {
			return "", err
		}

		vals = append(vals, op.expr)
		widths = append(widths, strconv.FormatUint(uint64(w), 10))
	}

	g.useHelper(helperCatU64)

	return fmt.Sprintf("catU64([]uint64{%s}, []uint{%s})", strings.Join(vals, ", "), strings.Join(widths, ", ")), nil
}

// emitPrintfHelpers writes the printf support helpers actually referenced by
// the generated code: the buffered stderr writer (flushed by Run), the
// width-padding writer dbgPad, the narrow-integer (dbgU) and big.Int (dbgB)
// formatters for the hot DEBUG path, the big.Int limb folders (u128/catU64) and
// chr (%c in a formatted FAIL).
func (g *generator) emitPrintfHelpers(c *code) {
	if g.usesHelper(helperDbgWriter) {
		c.line("// dbgw buffers printf output; Run flushes it. Per-instruction printf is")
		c.line("// the hot path of a non-quiet run, so an unbuffered os.Stderr write (a")
		c.line("// syscall each) would dominate; one bufio.Writer amortises that.")
		c.line("var dbgw = bufio.NewWriter(os.Stderr)")
		c.line("")
	}

	if g.usesHelper(helperDbgU) || g.usesHelper(helperDbgB) {
		c.line("// dbgPad left-pads digits d to width with pad, then writes them, matching")
		c.line("// the reference digit-only width padding (no base prefix, no sign).")
		c.line("func dbgPad(d []byte, width int, pad byte) {")
		c.line("for n := width - len(d); n > 0; n-- {")
		c.line("dbgw.WriteByte(pad)")
		c.line("}")
		c.line("dbgw.Write(d)")
		c.line("}")
		c.line("")
	}

	if g.usesHelper(helperDbgU) {
		c.line("// dbgU writes v in the given base, left-padded (narrow printf integers).")
		c.line("func dbgU(v uint64, base, width int, pad byte) {")
		c.line("var b [64]byte")
		c.line("dbgPad(strconv.AppendUint(b[:0], v, base), width, pad)")
		c.line("}")
		c.line("")
	}

	if g.usesHelper(helperDbgB) {
		c.line("// dbgB writes a big.Int in the given base, left-padded (wide/multi args).")
		c.line("func dbgB(v *big.Int, base, width int, pad byte) {")
		c.line("dbgPad(v.Append(nil, base), width, pad)")
		c.line("}")
		c.line("")
	}

	if g.usesHelper(helperChr) {
		c.line("// chr renders a value as a single raw byte (printf %c in a formatted FAIL).")
		c.line("func chr(v uint64) string { return string([]byte{byte(v)}) }")
		c.line("")
	}

	if g.usesHelper(helperU128) {
		c.line("// u128 folds a low/high pair into a big.Int for printf formatting.")
		c.line("func u128(lo, hi uint64) *big.Int {")
		c.line("return new(big.Int).Or(new(big.Int).Lsh(new(big.Int).SetUint64(hi), 64), new(big.Int).SetUint64(lo))")
		c.line("}")
		c.line("")
	}

	if g.usesHelper(helperCatU64) {
		c.line("// catU64 concatenates limbs (least-significant first) into a big.Int.")
		c.line("func catU64(vals []uint64, widths []uint) *big.Int {")
		c.line("v := new(big.Int)")
		c.line("for i := len(vals) - 1; i >= 0; i-- {")
		c.line("v.Lsh(v, widths[i])")
		c.line("v.Or(v, new(big.Int).SetUint64(vals[i]))")
		c.line("}")
		c.line("return v")
		c.line("}")
		c.line("")
	}
}
