// Command gogendump produces the artefacts behind perf_gogen.html:
//
//	gogendump artifacts <dir>  — generated Go (both shapes) + JSON inputs for keccak v2
//	gogendump keccak-steps     — micro-instruction counts for keccak v2 (linearity check)
//	gogendump sweep-steps      — micro-instruction counts for the sweep programs (size tuning)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/source"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/compiler/codegen"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
)

const keccakSrc = "testdata/zkc/bench/keccakf_v2.zkc"

func main() {
	switch os.Args[1] {
	case "artifacts":
		artifacts(os.Args[2])
	case "keccak-steps":
		keccakSteps()
	case "sweep-steps":
		sweepSteps()
	default:
		panic("unknown command " + os.Args[1])
	}
}

func artifacts(outDir string) {
	must(os.MkdirAll(outDir, 0o755))

	for _, shape := range []struct {
		name    string
		lowered bool
	}{{"plain", false}, {"lowered", true}} {
		wm := compile(keccakSrc, shape.lowered)

		src, err := vm.GenerateGo(wm, vm.GoGenConfig{})
		must(err)
		must(os.WriteFile(filepath.Join(outDir, "keccak_"+shape.name+".go.txt"), []byte(src), 0o644))

		fmt.Printf("%s: %d bytes of generated Go\n", shape.name, len(src))
	}

	// Input files at various block counts.
	for _, n := range []int{1, 500, 5000, 50000} {
		in := syntheticKeccakInput(n)
		wm := compile(keccakSrc, false)

		words, errs := vm.DecodeInputs(wm, in)
		if len(errs) > 0 {
			panic(fmt.Sprint(errs))
		}

		u64s := map[string][]uint64{}
		for name, vs := range words {
			us := make([]uint64, len(vs))
			for i, v := range vs {
				us[i] = v.Uint64()
			}
			u64s[name] = us
		}

		data, err := json.Marshal(u64s)
		must(err)
		must(os.WriteFile(filepath.Join(outDir, fmt.Sprintf("input_%d.json", n)), data, 0o644))
		fmt.Printf("input_%d.json: %d bytes\n", n, len(data))
	}
}

// keccakSteps prints executed micro-instruction counts for keccak v2 at several
// block counts, checking that the per-block marginal cost is constant (it is:
// keccak-f is data-independent), so large-input counts can be projected.
func keccakSteps() {
	fmt.Println("plain shape (counted on the uint64-narrowed machine):")

	var prev uint

	for _, n := range []int{1, 2, 500, 5000} {
		steps := countSteps(compile(keccakSrc, false), syntheticKeccakInput(n), true)
		marginal := ""

		if prev > 0 {
			marginal = fmt.Sprintf("  (marginal since prev: %d/blk)", steps-prev)
		}

		fmt.Printf("  %6d blocks: %12d instrs%s\n", n, steps, marginal)
		prev = steps
	}

	fmt.Println("lowered shape (counted on the Uint machine):")

	prev = 0

	for _, n := range []int{1, 2, 10} {
		steps := countSteps(compile(keccakSrc, true), syntheticKeccakInput(n), false)
		marginal := ""

		if prev > 0 {
			marginal = fmt.Sprintf("  (marginal since prev: %d/blk)", steps-prev)
		}

		fmt.Printf("  %6d blocks: %12d instrs%s\n", n, steps, marginal)
		prev = steps
	}
}

// sweepSteps prints micro-instruction counts for the sweep benchmark programs
// at candidate input sizes, on both shapes.
func sweepSteps() {
	for _, p := range []struct {
		name  string
		src   string
		sizes []int
		input func(int) map[string][]byte
	}{
		{"sort", "testdata/zkc/bench/sort.zkc", []int{1000, 5000, 20000}, syntheticSortInput},
		{"fnv1a", "testdata/zkc/bench/fnv1a_hash.zkc", []int{1000, 10000, 100000}, syntheticFnv1aInput},
		{"blake", "testdata/zkc/bench/blake.zkc", []int{100, 1000, 10000}, syntheticBlakeInput},
	} {
		for _, n := range p.sizes {
			plain := countSteps(compile(p.src, false), p.input(n), false)
			lowered := countSteps(compile(p.src, true), p.input(n), false)
			fmt.Printf("%-6s n=%-7d plain %12d instrs   lowered %12d instrs   (x%.1f)\n",
				p.name, n, plain, lowered, float64(lowered)/float64(plain))
		}
	}
}

// compile compiles a .zkc source into a fresh, vectorised word machine over
// Uint; `lowered` selects the prover shape.
func compile(path string, lowered bool) *vm.WordMachine[vm.Uint] {
	data, err := os.ReadFile(path)
	must(err)

	src := source.NewSourceFile(path, data)

	program, _, errs := compiler.Compile(field.KOALABEAR_16, *src)
	if len(errs) > 0 {
		panic(fmt.Sprint(errs))
	}

	cfg := codegen.DEFAULT_CONFIG.Field(field.KOALABEAR_16).LowerNatives(lowered).Vectorize(true).Quiet(true)

	wm, errs := program.Compile(cfg)
	if len(errs) > 0 {
		panic(fmt.Sprint(errs))
	}

	return wm
}

// countSteps boots and runs the machine, returning the executed
// micro-instruction count; narrow selects the (faster) uint64-narrowed
// interpreter, which executes the identical instruction stream.
func countSteps(wm *vm.WordMachine[vm.Uint], in map[string][]byte, narrow bool) uint {
	if narrow {
		return runCount(vm.WordToWordMachine[vm.Uint, vm.Uint64](wm), in)
	}

	return runCount(wm, in)
}

func runCount[W vm.Word[W], C vm.Core[W]](m C, in map[string][]byte) uint {
	words, errs := vm.DecodeInputs(m, in)
	if len(errs) > 0 {
		panic(fmt.Sprint(errs))
	}

	if err := m.Boot("main", words); err != nil {
		panic(err)
	}

	steps, err := vm.ExecuteAll(m, 1<<22)
	if err != nil {
		panic(err)
	}

	return steps
}

func syntheticKeccakInput(nBlocks int) map[string][]byte {
	nb := make([]byte, 8)
	nb[6] = byte(nBlocks >> 8)
	nb[7] = byte(nBlocks)
	blocks := make([]byte, nBlocks*17*8)
	for i := range blocks {
		blocks[i] = byte(i)
	}

	return map[string][]byte{"n_blocks": nb, "blocks": blocks}
}

// syntheticSortInput builds n pseudo-random bytes for the quicksort program.
func syntheticSortInput(n int) map[string][]byte {
	length := []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	data := make([]byte, n)

	for i := range data {
		data[i] = byte(i*7 + 13)
	}

	return map[string][]byte{"data_len": length, "data_in": data}
}

// syntheticFnv1aInput builds a 4-byte big-endian length header plus n bytes to
// hash.
func syntheticFnv1aInput(n int) map[string][]byte {
	data := make([]byte, 4+n)
	data[0], data[1], data[2], data[3] = byte(n>>24), byte(n>>16), byte(n>>8), byte(n)

	for i := 4; i < len(data); i++ {
		data[i] = byte(i * 31)
	}

	return map[string][]byte{"data": data}
}

// syntheticBlakeInput builds a blake2f input with r rounds (the EIP-152 vector
// geometry: 8x u64 state, 16x u64 message, 2x u64 counter, final flag).
func syntheticBlakeInput(rounds int) map[string][]byte {
	r := []byte{byte(rounds >> 24), byte(rounds >> 16), byte(rounds >> 8), byte(rounds)}
	h := make([]byte, 8*8)
	m := make([]byte, 16*8)
	t := make([]byte, 2*8)

	for i := range h {
		h[i] = byte(i*89 + 7)
	}

	for i := range m {
		m[i] = byte(i*57 + 3)
	}

	t[7] = 128 // t0 = message length

	return map[string][]byte{"r": r, "h": h, "m": m, "t": t, "f": {1}}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
