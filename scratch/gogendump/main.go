// Command gogendump generates the keccak v2 gogen artefacts (plain and
// lowered shapes) plus matching JSON input files, so the generated binary can
// be timed and profiled outside the Go benchmark harness.
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
	outDir := os.Args[1]
	must(os.MkdirAll(outDir, 0o755))

	if outDir == "steps" {
		for _, n := range []int{1, 2} {
			fmt.Printf("plain   %d blocks: %d steps\n", n, countSteps(compile(false), n))
			fmt.Printf("lowered %d blocks: %d steps\n", n, countSteps(compile(true), n))
		}

		return
	}

	for _, shape := range []struct {
		name    string
		lowered bool
	}{{"plain", false}, {"lowered", true}} {
		wm := compile(shape.lowered)

		src, err := vm.GenerateGo(wm, vm.GoGenConfig{})
		must(err)
		must(os.WriteFile(filepath.Join(outDir, "keccak_"+shape.name+".go.txt"), []byte(src), 0o644))

		fmt.Printf("%s: %d bytes of generated Go\n", shape.name, len(src))
	}

	// Input files at various block counts.
	for _, n := range []int{1, 500, 5000, 50000} {
		in := syntheticKeccakInput(n)
		wm := compile(false)

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

func compile(lowered bool) *vm.WordMachine[vm.Uint] {
	data, err := os.ReadFile(keccakSrc)
	must(err)

	src := source.NewSourceFile(keccakSrc, data)

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

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// countSteps boots and runs the machine, returning the executed step count.
func countSteps(wm *vm.WordMachine[vm.Uint], nBlocks int) uint {
	in := syntheticKeccakInput(nBlocks)

	words, errs := vm.DecodeInputs(wm, in)
	if len(errs) > 0 {
		panic(fmt.Sprint(errs))
	}

	if err := wm.Boot("main", words); err != nil {
		panic(err)
	}

	steps, err := vm.ExecuteAll(wm, 1<<22)
	if err != nil {
		panic(err)
	}

	return steps
}
