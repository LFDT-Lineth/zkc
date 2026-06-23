package picus

import (
	"math/big"

	"github.com/LFDT-Lineth/zkc/pkg/util/field"
)

// ModulusOf gets the modulus of F.
func ModulusOf[F field.Element[F]]() *big.Int {
	var z F
	return z.Modulus()
}

// Zero gets the additive identity of F.
func Zero[F field.Element[F]]() F {
	var z F
	return z.SetUint64(0)
}

// MaxValueBig returns (1<<bitwidth) - 1 as a `*big.Int`.
func MaxValueBig(bitwidth int) *big.Int {
	if bitwidth < 0 {
		panic("bitwidth must be non-negative")
	}

	if bitwidth == 0 {
		return new(big.Int) // 0
	}

	one := big.NewInt(1)

	return new(big.Int).Sub(new(big.Int).Lsh(one, uint(bitwidth)), one)
}
