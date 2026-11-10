package zkc

// Preliminary evidence (not for merge) for the halt-counting argument on
// jmp.zkc: for the #[global] function step, bus balance gives
// #halt rows (H) = #calls from main (M). Each case builds the step module
// from a list of rows: an honest row (by index) or the forged self-loop row
// at pc = 3, then runs zkc's AIR checker.
//
// Run (from the repo root):
//   go run ./cmd/zkc compile -o /tmp/jmp.bin <path>/jmp.zkc
//   SL_BIN=/tmp/jmp.bin go test -count=1 -run TestZZHaltCount -v ./pkg/cmd/zkc/
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

type hF = koalabear.Element

const loopRow = -1

func TestZZHaltCount(t *testing.T) {
	if os.Getenv("SL_BIN") == "" {
		t.Skip("set SL_BIN to the output of: zkc compile -o jmp.bin jmp.zkc")
	}
	binf := ReadBinaryFile[hF, vm.Uint32](os.Getenv("SL_BIN"))
	in, _ := zutil.ParseJsonInputFile([]byte(`{"next":"0x0102ff03"}`))
	ch := checker.New(binf.AirConstraints())
	_, lt, errs := binf.Trace(in, vm.DEFAULT_TRACE_CONFIG)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	sh, _ := lt.Unwrap().Get(0)
	honest := sh.Unwrap()
	k := koalabear.New
	loop := map[string]hF{
		"pc": k(3), "n": k(3), "$2": k(0), "fsc$3": k(1), "$ret": k(1),
		"$lookup_sel_5": k(0), "$lookup_sel_6": k(1),
		"(inv (- 255 step:n))": k(255).Sub(k(3)).Inverse(),
	}
	// build: main active or not; step rows from a list (0 = honest padding
	// row, 1..3 = honest active rows, loopRow = forged self-loop).
	build := func(mainActive bool, rows []int) trace.Shard[hF] {
		mods := make([]trace.Module[hF], honest.Width())
		for m := range honest.Width() {
			mod := honest.Module(m)
			if mod.Width() == 0 || mod.Column(0) == nil {
				mods[m] = mod
				continue
			}
			cols := make([]array.Array[hF], mod.Width())
			for c := range mod.Width() {
				col, name := mod.Column(c), mod.Descriptor().Columns[c].Name
				var vals []hF
				switch mod.Name() {
				case "main":
					for r := range col.Len() {
						if mainActive {
							vals = append(vals, col.Get(r))
						} else {
							vals = append(vals, k(0))
						}
					}
				case "step":
					for _, src := range rows {
						if src == loopRow {
							vals = append(vals, loop[name])
						} else {
							vals = append(vals, col.Get(uint(src)))
						}
					}
				default: // in/next, out: keep honest (out[0] = 2, written by the halt row)
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
	check := func(label string, s trace.Shard[hF]) {
		lz := trace.NewLazyTrace[hF](func() (util.Option[trace.Shard[hF]], []error) { return util.Some(s), nil })
		fails, errs := ch.CheckLazy(lz)
		t.Logf("%s: %d failures, %d errors", label, len(fails), len(errs))
		for i, f := range fails {
			if i < 4 {
				t.Logf("   %v", f)
			}
		}
	}
	check("A honest:                 main, step rows [pad,0,1,2]          (H=1,M=1)", build(true, []int{0, 1, 2, 3}))
	check("B forged self-loop:       no main, [pad,pad,pad,loop]          (H=0,M=0)", build(false, []int{0, 0, 0, loopRow}))
	check("C loop + halt, no main:   no main, [pad,pad,loop,halt(pc=2)]   (H=1,M=0)", build(false, []int{0, 0, loopRow, 3}))
	check("D honest + extra loop:    main, [pad x4,loop,0,1,2]            (H=1,M=1)", build(true, []int{0, 0, 0, 0, loopRow, 1, 2, 3}))
	check("E three halts, one main:  main, [pad x3,0,1,2,2,2]            (H=3,M=1)", build(true, []int{0, 0, 0, 1, 2, 3, 3, 3}))
}
