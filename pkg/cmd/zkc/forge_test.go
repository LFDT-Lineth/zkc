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
