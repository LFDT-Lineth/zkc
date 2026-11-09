package zkc

// Preliminary evidence (not for merge): zkc's AIR checker accepts a forged
// trace of fib(10) in which no function ever runs (every main/fib cell is
// zero) and the public output is 0x99 instead of the honest 0x37.
//
// Run:
//   make build
//   ./bin/zkc compile -o /tmp/forge_fib.bin pkg/cmd/zkc/testdata/forge_fib.zkc
//   FORGE_BIN=/tmp/forge_fib.bin go test -count=1 -run TestForgedTraceAccepted -v ./pkg/cmd/zkc/
//
// The test FAILS while the soundness gap exists: it asserts that the checker
// rejects the forged trace.

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

type forgeF = koalabear.Element

func TestForgedTraceAccepted(t *testing.T) {
	binPath := os.Getenv("FORGE_BIN")
	if binPath == "" {
		t.Skip("set FORGE_BIN to the output of: zkc compile -o forge_fib.bin pkg/cmd/zkc/testdata/forge_fib.zkc")
	}
	binf := ReadBinaryFile[forgeF, vm.Uint32](binPath)
	in, _ := zutil.ParseJsonInputFile([]byte(`{"in":"0x0a"}`))
	ch := checker.New(binf.AirConstraints())
	check := func(s trace.Shard[forgeF]) int {
		lz := trace.NewLazyTrace[forgeF](func() (util.Option[trace.Shard[forgeF]], []error) { return util.Some(s), nil })
		fails, errs := ch.CheckLazy(lz)
		return len(fails) + len(errs)
	}
	// 1. Honest trace of fib(10): must be accepted (out[0] = 0x37).
	_, lt, errs := binf.Trace(in, vm.DEFAULT_TRACE_CONFIG)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	sh, _ := lt.Unwrap().Get(0)
	honest := sh.Unwrap()
	if n := check(honest); n != 0 {
		t.Fatalf("honest trace rejected with %d failures", n)
	}
	// 2. Forge: zero every main/fib cell (no function ever runs) and set the
	//    last row of the public output column out.f'0 to 0x99.
	mods := make([]trace.Module[forgeF], honest.Width())
	for m := range honest.Width() {
		mod := honest.Module(m)
		if mod.Width() == 0 || mod.Column(0) == nil { // static tables: keep
			mods[m] = mod
			continue
		}
		cols := make([]array.Array[forgeF], mod.Width())
		for c := range mod.Width() {
			col := mod.Column(c)
			vals := make([]forgeF, col.Len())
			for r := range col.Len() {
				vals[r] = col.Get(r)
				if mod.Name() == "main" || mod.Name() == "fib" {
					vals[r] = koalabear.New(0)
				}
			}
			if mod.Name() == "out" && mod.Descriptor().Columns[c].Name == "f'0" {
				t.Logf("honest out.f'0 = %v", col)
				vals[col.Len()-1] = koalabear.New(0x99)
			}
			cols[c] = array.NewStaticArray(col.BitWidth(), vals...)
		}
		mods[m] = trace.NewModule(mod.Descriptor(), cols...)
	}
	forged := trace.NewShard(mods)
	// 3. The forged trace claims fib(10) = 0x99 with zero execution rows.
	if n := check(forged); n == 0 {
		t.Errorf("SOUNDNESS GAP: forged trace (no main/fib execution, out[0]=0x99) accepted with 0 failures")
	} else {
		t.Logf("forged trace rejected with %d failures", n)
	}
}

// does "$ret[N]=1 on the shard function" (#2305
// proposal) force main to run? Forge a trace where main never runs and one
// step row at the self-loop entry next[3]=3 sends and receives step(3).
func TestZZSelfLoop(t *testing.T) {
	binf := ReadBinaryFile[forgeF, vm.Uint32](os.Getenv("SL_BIN"))
	in, _ := zutil.ParseJsonInputFile([]byte(`{"next":"0x0102ff03"}`))
	ch := checker.New(binf.AirConstraints())
	check := func(label string, s trace.Shard[forgeF]) {
		lz := trace.NewLazyTrace[forgeF](func() (util.Option[trace.Shard[forgeF]], []error) { return util.Some(s), nil })
		fails, errs := ch.CheckLazy(lz)
		t.Logf("%s: %d failures, %d errors", label, len(fails), len(errs))
		for i, f := range fails {
			if i < 3 {
				t.Logf("   %v", f)
			}
		}
	}
	_, lt, errs := binf.Trace(in, vm.DEFAULT_TRACE_CONFIG)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	sh, _ := lt.Unwrap().Get(0)
	honest := sh.Unwrap()
	check("honest", honest)
	// The forged step row: pc=3, n=next[3]=3, takes the "call step!(n)" branch.
	k := koalabear.New
	row := map[string]forgeF{
		"pc": k(3), "n": k(3), "$2": k(0), "fsc$3": k(1), "$ret": k(1),
		"$lookup_sel_5": k(0), "$lookup_sel_6": k(1),
		"(inv (- 255 step:n))": k(255).Sub(k(3)).Inverse(),
	}
	forge := func(outVal uint32) trace.Shard[forgeF] {
		mods := make([]trace.Module[forgeF], honest.Width())
		for m := range honest.Width() {
			mod := honest.Module(m)
			if mod.Width() == 0 || mod.Column(0) == nil {
				mods[m] = mod
				continue
			}
			cols := make([]array.Array[forgeF], mod.Width())
			for c := range mod.Width() {
				col, name := mod.Column(c), mod.Descriptor().Columns[c].Name
				vals := make([]forgeF, col.Len())
				for r := range col.Len() {
					vals[r] = col.Get(r)
				}
				last := int(col.Len()) - 1
				switch mod.Name() {
				case "main": // main never runs: all padding
					for r := range vals {
						vals[r] = k(0)
					}
				case "step": // padding rows = honest row 0; last row = self-loop
					for r := 0; r < last; r++ {
						vals[r] = col.Get(0)
					}
					vals[last] = row[name]
				case "out":
					if name == "f" {
						vals[last] = k(outVal)
					}
				}
				cols[c] = array.NewStaticArray(32, vals...)
			}
			mods[m] = trace.NewModule(mod.Descriptor(), cols...)
		}
		return trace.NewShard(mods)
	}
	f := forge(0x99)
	for m := range f.Width() {
		if mod := f.Module(m); mod.Name() == "main" || mod.Name() == "step" || mod.Name() == "out" {
			for c := range mod.Width() {
				t.Logf("forged %s.%s = %v", mod.Name(), mod.Descriptor().Columns[c].Name, mod.Column(c))
			}
		}
	}
	check("forged: main never runs, step(3)->step(3), out=0x99", f)
	// Control: same shape at pc=1 (next[1]=2): receives step(1), sends step(2).
	row["pc"], row["n"], row["(inv (- 255 step:n))"] = k(1), k(2), k(253).Inverse()
	check("control: main never runs, step(1)->step(2)", forge(0x99))
}

//	self-loop forgery on the interpreter-shaped toy
//
// jmpwb.zkc, where the call message also carries the regs memory stamp.
func TestZZSelfLoopWB(t *testing.T) {
	binf := ReadBinaryFile[forgeF, vm.Uint32](os.Getenv("SL_BIN"))
	in, _ := zutil.ParseJsonInputFile([]byte(`{"next":"0x0102ff03","wb":"0x01000100"}`))
	ch := checker.New(binf.AirConstraints())
	check := func(label string, s trace.Shard[forgeF]) {
		lz := trace.NewLazyTrace[forgeF](func() (util.Option[trace.Shard[forgeF]], []error) { return util.Some(s), nil })
		fails, errs := ch.CheckLazy(lz)
		t.Logf("%s: %d failures, %d errors", label, len(fails), len(errs))
		for i, f := range fails {
			if i < 3 {
				t.Logf("   %v", f)
			}
		}
	}
	_, lt, errs := binf.Trace(in, vm.DEFAULT_TRACE_CONFIG)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	sh, _ := lt.Unwrap().Get(0)
	honest := sh.Unwrap()
	check("honest", honest)
	k := koalabear.New
	// forge copies honest step row `tmpl`, retargets it to pc = n = target,
	// and makes it the only active step row. main and regs become padding.
	forge := func(tmpl int, target uint32) trace.Shard[forgeF] {
		mods := make([]trace.Module[forgeF], honest.Width())
		for m := range honest.Width() {
			mod := honest.Module(m)
			if mod.Width() == 0 || mod.Column(0) == nil {
				mods[m] = mod
				continue
			}
			cols := make([]array.Array[forgeF], mod.Width())
			for c := range mod.Width() {
				col, name := mod.Column(c), mod.Descriptor().Columns[c].Name
				vals := make([]forgeF, col.Len())
				last := len(vals) - 1
				for r := range vals {
					vals[r] = col.Get(uint(r))
				}
				switch mod.Name() {
				case "main": // main never runs: all zero
					for r := range vals {
						vals[r] = k(0)
					}
				case "regs": // no memory accesses: honest padding row 0
					for r := range vals {
						vals[r] = col.Get(0)
					}
				case "step":
					for r := 0; r < last; r++ {
						vals[r] = col.Get(0)
					}
					vals[last] = col.Get(uint(tmpl))
					switch name {
					case "pc", "n":
						vals[last] = k(target)
					case "(inv (- 255 step:n))":
						vals[last] = k(255).Sub(k(target)).Inverse()
					}
				case "out":
					if name == "f" {
						vals[last] = k(0x99)
					}
				}
				cols[c] = array.NewStaticArray(32, vals...)
			}
			mods[m] = trace.NewModule(mod.Descriptor(), cols...)
		}
		return trace.NewShard(mods)
	}
	f := forge(2, 3) // template: honest pc=1 row (wb=0, no write); "j ." at entry 3
	for m := range f.Width() {
		if mod := f.Module(m); mod.Name() == "main" || mod.Name() == "step" {
			for c := range mod.Width() {
				t.Logf("forged %s.%s = %v", mod.Name(), mod.Descriptor().Columns[c].Name, mod.Column(c))
			}
		}
	}
	check("forged: j . (no write), main never runs, out=0x99", f)
	// Control: a write-back row (honest pc=0, wb=1) retargeted to a self-loop.
	// Its stamp goes in=1, out=2, so the message cannot match itself.
	check("control: self-loop on a write-back row", forge(1, 3))
}
