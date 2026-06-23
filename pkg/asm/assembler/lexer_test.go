package assembler

import (
	"testing"

	"github.com/LFDT-Lineth/zkc/pkg/util/assert"
)

func TestLexerNumberRule(t *testing.T) {
	assert.Equal(t, 1, number([]int32{'0', 'b'})) // taken as the decimal number 0.
	assert.Equal(t, 4, number([]int32{'0', 'b', '1', '0', 'a'}))
	assert.Equal(t, 3, number([]int32{'0', 'x', '1', 'p'}))
	assert.Equal(t, 2, number([]int32{'1', '2', 'a'}))
	assert.Equal(t, 5, number([]int32{'0', 'x', 'A', '_', '0', '='}))
}
