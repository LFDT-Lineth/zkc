package main

import (
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/consensys/bavard"
)

const copyrightHolder = "Consensys Incorporated"

//go:generate go run main.go
func main() {
	bgen := bavard.NewBatchGenerator(copyrightHolder, 2025, "go-corset")

	specs := []fieldSpecs{
		{Name: "mersenne31", Modulus: 1<<31 - 1},
		{Name: "gf251", Modulus: 251},
		{Name: "gf8209", Modulus: 8209},
		{Name: "koalabear", Modulus: 1<<31 - 1<<24 + 1},
	}

	for _, spec := range specs {
		cfg, err := spec.config()
		assertNoError(err, "for field \"%s\"", spec.Name)

		assertNoError(bgen.Generate(cfg, spec.Name, "templates",
			bavard.Entry{
				File:      fmt.Sprintf("../../%s/element.go", spec.Name),
				Templates: []string{"element.go.tmpl"},
				BuildTag:  "", // TODO remove
			},
			bavard.Entry{
				File:      fmt.Sprintf("../../%s/element_test.go", spec.Name),
				Templates: []string{"element.test.go.tmpl"},
				BuildTag:  "", // TODO remove
			},
		), "for field \"%s\"", spec.Name)
	}
	// run gofmt on whole directory
	runCmd("gofmt", "-w", "../../../")

	// run goimports on whole directory
	runCmd("goimports", "-w", "../../../")
}

func runCmd(name string, arg ...string) {
	fmt.Println(name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	assertNoError(cmd.Run(), "")
}

type fieldSpecs struct {
	Name    string
	Modulus uint32
}

type fieldConfig struct {
	fieldSpecs
	RSqModM           uint32
	RModM             uint32
	NegModulusInvModR uint32
}

func (f fieldSpecs) config() (*fieldConfig, error) {
	const R = 1 << 32

	specs := fieldConfig{
		fieldSpecs: f,
	}

	if f.Modulus >= R>>1 { // need an extra bit
		return nil, fmt.Errorf("modulus must be less than 2³¹")
	}

	m := big.NewInt(int64(f.Modulus))
	r := big.NewInt(R)

	var x big.Int

	x.Mod(r, m)
	specs.RModM = uint32(x.Uint64())

	x.Mul(&x, &x).
		Mod(&x, m)

	specs.RSqModM = uint32(x.Uint64())

	x.ModInverse(m, r)
	specs.NegModulusInvModR = uint32(R - x.Uint64())

	return &specs, nil
}

func assertNoError(err error, contextAndArgs ...any) {
	if err != nil {
		msg := err.Error()

		if len(contextAndArgs) > 0 {
			allArgs := append(slices.Clone(contextAndArgs[1:]), err)
			msg = fmt.Sprintf(contextAndArgs[0].(string)+": %v", allArgs...)
		}

		fmt.Println(msg)
		os.Exit(1)
	}
}
