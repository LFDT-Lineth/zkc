package zkc

// Preliminary evidence (not for merge) for the halt-counting argument on
// jmp.zkc (see issues #2305 / #2306).
//
// The program under test:
//
//	pub input next(addr:u8) -> (pc:u8)
//	pub output out(addr:u1) -> (f:u8)
//	fn main() { step!(0) }
//	#[global]
//	fn step(pc:u8) -> ! {
//	    var n:u8 = next[pc]
//	    if n == 255 { out[0] = pc; done }   // the "halt" row
//	    step!(n)
//	}
//
// with input next = [1, 2, 255, 3]. The honest run is
// main -> step(0) -> step(1) -> step(2), which halts and writes out[0] = 2.
// Entry 3 (next[3] = 3) is a self-loop that the honest run never reaches.
//
// Why halt counting works. step is #[global], so every call to it is a
// message on the bus "bus_step":
//   - each active step row RECEIVES its own call once          -> A receives
//   - each active step row SENDS the next call, unless it halts -> A - H sends
//   - each active main row SENDS the first call step!(0)        -> M sends
//
// The bus must balance (sends = receives), so A = (A - H) + M, i.e. H = M:
// the number of halt rows equals the number of calls from main.
//
// Each case below builds a step module from a chosen list of rows (honest
// rows picked by index, or the forged self-loop row) and runs zkc's AIR
// checker on the result.
//
// Run (from the repo root):
//   go run ./cmd/zkc compile -o /tmp/jmp.bin <path>/jmp.zkc
//   SL_BIN=/tmp/jmp.bin go test -count=1 -run TestZZHaltCount -v ./pkg/cmd/zkc/
//
// Use a jmp.bin compiled by this same checkout: a binary from another zkc
// version fails to load with "EOF".
//
// Expected: A, B, D pass (H = M); C and E fail with bus_step unbalanced,
// tally M - H.

import (
	"os"
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/schema/constraint/checker"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	zutil "github.com/LFDT-Lineth/zkc/pkg/zkc/util"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

// hF is the field the checker runs over (KOALABEAR_16).
type hF = koalabear.Element

// loopRow is a marker in a row list meaning "insert the forged self-loop
// row here" instead of copying an honest row.
const loopRow = -1

// This test deliberately forges execution traces to investigate whether the AIR constraints enforce
// that every halt is accounted for by a corresponding call from main, even when fake self-loops are inserted.
func TestZZHaltCount(t *testing.T) {
	if os.Getenv("SL_BIN") == "" {
		t.Skip("set SL_BIN to the output of: zkc compile -o jmp.bin jmp.zkc")
	}

	// 1. Load the compiled program and run it honestly on
	//    next = [1, 2, 255, 3] (packed as the bytes 0x01 0x02 0xff 0x03).
	binf := ReadBinaryFile[hF, vm.Uint32](os.Getenv("SL_BIN"))
	in, _ := zutil.ParseJsonInputFile([]byte(`{"next":"0x0102ff03"}`))

	// The checker holds the program's AIR constraints. Every forged trace
	// below is checked against these same constraints.
	ch := checker.New(binf.AirConstraints())

	// `lt` is the honest trace, which we use as a template for the forged traces.
	_, lt, errs := binf.Trace(in, vm.DEFAULT_TRACE_CONFIG)
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	// The honest run fits in one shard. Its modules (main, step, next, out)
	// are the template we copy rows from.
	shard, _ := lt.Unwrap().Get(0)
	honestShard := shard.Unwrap()

	// 2. The forged self-loop row: step(3) reads next[3] = 3 and calls
	//    step!(3). The call it sends equals the call it receives, so this
	//    row balances the bus on its own and needs no caller.
	//    Values are given per column of the step module:

	// The important fields:
	// - "pc": 3 — the program counter is 3.
	// - "n": 3 — the next program counter is also 3.
	// - "$ret": 1 — return-related flag is enabled.
	// - The lookup selector fields choose particular lookup behavior.
	// - The inverse expression supplies a field inverse needed by a constraint.
	// The important trick is pc = n = 3.

	// The row claims to be a self-loop: execution stays at the same PC.
	// This is a forged row, not a row produced by the VM.
	// The test wants to see whether the AIR constraints allow this row to coexist with the rest of the trace.
	k := koalabear.New
	loop := map[string]hF{
		// this row executes step(3)
		"pc": k(3),

		// n = next[3] = 3, so the next call is step!(3)
		"n": k(3),

		// Compiler-generated control columns, set as on an honest
		// non-halting row so the row takes the "n != 255 -> step!(n)" branch.
		"$2": k(0), "fsc$3": k(1),
		// row is active: its bus messages count
		"$ret": k(1),

		// Lookup selectors: 5 = write into out (off: the row does not halt),
		// 6 = read next[pc] (on: next[3] = 3 is checked against the input).
		"$lookup_sel_5": k(0), "$lookup_sel_6": k(1),
		// Inverse of (255 - n): proves n != 255, i.e. this is not a halt row.
		"(inv (- 255 step:n))": k(255).Sub(k(3)).Inverse(),
	}

	// 3. build constructs a new trace shard using two instructions from the caller:
	// - mainActive: should the main module be active? -
	// It keeps main's honest active row (M = 1), or zero it into padding (M = 0);
	// - rows: which rows should appear in the step module?
	//   Index 0 is the honest padding
	//   row; 1..3 are the honest active rows step(0), step(1), step(2) resp.
	//   (index 3 is the halt row); loopRow is the forged self-loop.
	//   All other modules (next, out) are kept honest, so out[0] = 2.
	build := func(mainActive bool, rows []int) trace.Shard[hF] {
		mods := make([]trace.Module[hF], honestShard.Width())
		for m := range honestShard.Width() {
			mod := honestShard.Module(m)
			// Modules without columns are copied unchanged.
			if mod.Width() == 0 || mod.Column(0) == nil {
				mods[m] = mod
				continue
			}

			// Process each column
			cols := make([]array.Array[hF], mod.Width())
			for c := range mod.Width() {
				col, name := mod.Column(c), mod.Descriptor().Columns[c].Name
				var vals []hF
				switch mod.Name() {
				// If mainActive is true, preserve the honest main rows.
				// Otherwise, replace every value in every main column with zero.
				// In other words, this can simulate a trace where main never calls step
				case "main":
					// main has one row. Copy it (M = 1), or zero every
					// column so it becomes inactive padding (M = 0).
					for r := range col.Len() {
						if mainActive {
							vals = append(vals, col.Get(r))
						} else {
							vals = append(vals, k(0))
						}
					}

				// For each requested row:
				// - If it is an ordinary index, copy that row from the honest trace.
				// - If it is loopRow, insert the manually forged self-loop value.
				// So the test can create traces containing arbitrary combinations of honest rows and fake rows.
				case "step":
					// Lay out step's rows in the order given by rows.
					for _, src := range rows {
						if src == loopRow {
							vals = append(vals, loop[name])
						} else {
							vals = append(vals, col.Get(uint(src)))
						}
					}

					// Keep the other modules unchanged.
					// The comment specifically mentions out: its first value is 2, written by the halt row.
					// This is important because the test is trying to isolate the behavior of main and step, rather than arbitrarily changing all the supporting data.
				default:
					// next, out: keep honest (out[0] = 2, written by the halt row)
					for r := range col.Len() {
						vals = append(vals, col.Get(r))
					}
				}
				cols[c] = array.NewStaticArray(32, vals...)
			}
			mods[m] = trace.NewModule(mod.Descriptor(), cols...)
		}
		return trace.NewShard(mods)
	}

	// check runs the AIR checker on the given shard and logs the verdict
	// plus the first few failing constraints.
	check := func(label string, s trace.Shard[hF]) {
		// Wrap the shard in a lazy trace so the checker can request it on demand.
		lz := trace.NewLazyTrace[hF](
			func() (util.Option[trace.Shard[hF]], []error) {
				return util.Some(s), nil
			})

		// fails means the trace violated AIR constraints.
		// errs means the checker encountered an error while checking.
		fails, errs := ch.CheckLazy(lz)
		t.Logf("%s: %d failures, %d errors", label, len(fails), len(errs))
		for i, f := range fails {
			if i < 4 {
				t.Logf("   %v", f)
			}
		}
	}

	// 4. The cases. H = halt rows, M = active main rows; the bus tally is
	//    M - H. Row lists are padded with honest padding rows (index 0) to a
	//    power-of-two height.

	/*

			main:
		  one active row ($ret = 1)
		       |
		       | sends call step(0)
		       v
		step:        #0    #1    #2    #3
		  pc          0     0     1     2
		  n           0     1     2   255
		  $ret        0     1     1     1
		              ^     ^     ^     ^
		              |     |     |     |
		           padding  |     |     |
		                  step(0) |     |
		                        step(1) |
		                              step(2), halt

	*/

	// A: the honest trace. One main call, one halt -> accepted.
	check("A honest:                 main, step rows [pad,0,1,2]          (H=1,M=1)", build(true, []int{0, 1, 2, 3}))

	// B: the attack. No main, only the self-loop. H = M = 0, so the bus
	//    still balances -> accepted. A "at least one halt" rule closes this.
	check("B forged self-loop:       no main, [pad,pad,pad,loop]          (H=0,M=0)", build(false, []int{0, 0, 0, loopRow}))

	// C: main as padding, plus a halt row. The halt receives a call that
	//    nobody sends (a loop only pays for its own receive) -> rejected,
	//    tally -1.
	check("C loop + halt, no main:   no main, [pad,pad,loop,halt(pc=2)]   (H=1,M=0)", build(false, []int{0, 0, loopRow, 3}))

	// D: honest run plus an extra self-loop row. The loop changes neither H  nor M -> accepted.
	// Halt counting cannot remove loops; the step counter (#2305) does.
	// We pad 4 zeros here so that trace has 2^3 = 8 rows. The fake loop takes one slot, and the three honest active rows take three slots.
	// That leaves four padding slots.
	check("D honest + extra loop:    main, [pad x4,loop,0,1,2]            (H=1,M=1)", build(true, []int{0, 0, 0, 0, loopRow, 1, 2, 3}))

	// E: three halt rows but one main call. Two halts lack a matching send
	//    -> rejected, tally 1 - 3 = -2.
	check("E three halts, one main:  main, [pad x3,0,1,2,2,2]            (H=3,M=1)", build(true, []int{0, 0, 0, 1, 2, 3, 3, 3}))
}
