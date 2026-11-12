// Copyright Consensys Incorporated
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
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/bit"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/descriptor"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// ErrMachineFailure is used to indicate when a gogen binary leads to a machine
// failure, as opposed to any other kind of internal error.
var ErrMachineFailure = errors.New("machine failure")

// Config controls the shape of the generated artefact.
type Config struct {
	// Package is the name of the generated Go package.  The package "main"
	// additionally gets a JSON stdin/stdout main() harness (used by Compile and
	// the differential tests), which is the only supported way of executing
	// the generated code.  Any other name yields a package without a harness
	// or entry point, useful only for inspecting the generated functions.
	Package string
	// NoIntervals disables the flow-sensitive interval analysis, falling back
	// to width-derived bounds only (more emitted checks, otherwise identical
	// semantics).  A debugging escape hatch.
	NoIntervals bool
	// Source, when non-empty, is recorded in the generated header (e.g.
	// "keccakf.zkc sha256:ab12…"), so go:generate workflows can detect a stale
	// artefact by comparing it against the current source.
	Source string
	// CheckPointing strategy
	CheckPointing util.Option[descriptor.ShardingStrategy]
}

// WithCheckPointing returns a config with the checkpointing strategy updated
// accordingly, but which is otherwise identical.
func (p Config) WithCheckPointing(strategy descriptor.ShardingStrategy) Config {
	p.CheckPointing = util.Some(strategy)
	//
	return p
}

// DEFAULT_CONFIG provides a default configuration config which is generally
// sufficient for small, self-contained tests.
var DEFAULT_CONFIG = Config{}

// Compile compiles a ZkC program into a GoGen program which can be
// executed, or an error indicating some kind of compilation failure.
func Compile(program descriptor.Program[word.Uint], config Config) (Binary, error) {
	// Generate native Go source for the word machine.
	var (
		src, err = Generate(program, config)
		dir      string
	)
	//
	if err != nil {
		return Binary{}, err
	}
	// Construct temporary directory in which to build
	dir, err = os.MkdirTemp("", "zkcgen")
	//
	if err != nil {
		return Binary{}, err
	}
	// Determine program path
	programPath := filepath.Join(dir, "prog")
	// Construct file holding contents of generated program.
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		//nolint:errcheck
		os.RemoveAll(dir)
		return Binary{}, err
	}
	// Write out go.mod
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module zkcgen\n\ngo 1.24\n"), 0o644); err != nil {
		//nolint:errcheck
		os.RemoveAll(dir)
		return Binary{}, err
	}
	//
	cmd := exec.Command("go", "build", "-o", programPath, ".")
	cmd.Dir = dir
	//
	if out, err := cmd.CombinedOutput(); err != nil {
		//nolint:errcheck
		os.RemoveAll(dir)
		return Binary{}, fmt.Errorf("go build failed: %v\n%s", err, out)
	}
	//
	return Binary{programPath}, nil
}

// ============================================================================
// GoGenProgram
// ============================================================================

// Binary provides a handle for a (compiled) GoGen program.
type Binary struct {
	// Path to the compiled executable
	path string
}

// Free any resources used by this handle, such as a physical executable stored
// on the file system.
func (p *Binary) Free() error {
	if p.path != "" {
		// Remove temp directory
		return os.RemoveAll(filepath.Dir(p.path))
	}
	//
	return nil
}

// Path returns the path of this binary executable.
func (p *Binary) Path() string {
	return p.path
}

// Run executes the compiled gogen binary on the given input, returning the
// output, and any errors arising.  Finally, any output from the program (e.g.
// from debug/print statements) is written to the given stderr writer.
func (p *Binary) Run(in map[string]bit.Array, stderr io.Writer) (output map[string]bit.Array, err error) {
	//
	output, _, err = p.CheckPoint(in, func(checkpoint.CheckPoint) {
		panic("checkpoint received")
	}, stderr)
	//
	return output, err
}

// CheckPoint executes the compiled gogen binary on the given input, returning
// the output, and any errors arising.  Furthermore, the consumer function is
// called for any checkpoints generated during the process.  Finally, any output
// from the program (e.g. from debug/print statements) is written to the given
// stderr writer.
func (p *Binary) CheckPoint(in map[string]bit.Array, consumer func(checkpoint.CheckPoint), stderr io.Writer,
) (output map[string]bit.Array, nCheckpoints uint, err error) {
	//
	var (
		input []byte
		// byteconsumer is responsible for converting a byte array into a
		// checkpoint, and passing it onto the checkpoint consumer.
		byteConsumer = func(line []byte) error {
			// Parse json
			if combo, err := unmarshalCombo(line); err != nil {
				return fmt.Errorf("decoding generated output %q: %v", string(line), err)
			} else if combo.HasSecond() {
				nCheckpoints++
				//
				consumer(combo.Second())
			} else {
				output = combo.First()
			}
			//
			return nil
		}
	)
	// Marshal input data into bytes
	if input, err = marshalInputs(in); err != nil {
		return nil, nCheckpoints, err
	} else if err = runRaw(p.path, input, byteConsumer, stderr); err != nil {
		return nil, nCheckpoints, err
	}
	// Decode final output
	return output, nCheckpoints, nil
}

// MarshalInputs renders input memories as the JSON the generated harness reads:
// one "0x…" hex string per memory.  Benchmarks pre-marshal once so the timed
// loop measures the executor, not the marshalling.
func marshalInputs(in map[string]bit.Array) ([]byte, error) {
	raw := make(map[string]string, len(in))
	for name, bits := range in {
		raw[name] = "0x" + bits.String()
	}

	return json.Marshal(raw)
}

func unmarshalCombo(line []byte) (util.Union[map[string]bit.Array, checkpoint.CheckPoint], error) {
	var (
		raw   map[string]string
		combo util.Union[map[string]bit.Array, checkpoint.CheckPoint]
		cp    checkpoint.CheckPoint
	)
	// Parse line
	if err := json.Unmarshal(line, &raw); err != nil {
		return combo, fmt.Errorf("decoding generated output %q: %v", string(line), err)
	}
	// Distinguish checkpoint from output
	if data, ok := raw["$"]; ok {
		// Parse input array
		bytes, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
		// Check for errors
		if err != nil {
			return combo, err
		} else if err = cp.Unmarshal(bytes); err != nil {
			return combo, err
		}
		//
		return util.Union2[map[string]bit.Array](cp), nil
	}
	// Parse output
	var out, err = unmarshalOutput(raw)
	//
	return util.Union1[map[string]bit.Array, checkpoint.CheckPoint](out), err
}

func unmarshalOutput(raw map[string]string) (map[string]bit.Array, error) {
	// Parse output
	out := make(map[string]bit.Array, len(raw))
	//
	for name, s := range raw {
		bytes, err := bit.ParseArray(strings.TrimPrefix(s, "0x"))
		//
		if err != nil {
			return out, fmt.Errorf("decoding generated output %q: %v", name, err)
		}
		//
		out[name] = bytes
	}
	// Done
	return out, nil
}

func runRaw(path string, input []byte, consumer func([]byte) error, stderr io.Writer) error {
	var (
		cmd    = exec.Command(path)
		stdout io.ReadCloser
		err    error
	)
	//
	cmd.Stdin = bytes.NewReader(input)
	cmd.Stderr = stderr
	// Construct output pipe
	if stdout, err = cmd.StdoutPipe(); err != nil {
		return err
	} else if err = cmd.Start(); err != nil {
		return err
	}
	// Process output lines
	if err = readLines(stdout, consumer); err != nil {
		// Nobody will read stdout anymore, so the process must be terminated
		// (otherwise it could block forever writing to a full pipe).  Wait is
		// then required to release the process and close the pipe.
		//nolint:errcheck
		cmd.Process.Kill()
		//nolint:errcheck
		cmd.Wait()
		//
		return err
	}
	// Wait for process to terminate
	var ee *exec.ExitError
	//
	if err = cmd.Wait(); errors.As(err, &ee) && ee.ExitCode() == 1 {
		// Catch machine failure
		return ErrMachineFailure
	}
	//
	return err
}

// readLines splits r into newline-terminated lines and passes each one to the
// given consumer, stopping at the first error.  Unlike bufio.Scanner, there is
// no upper bound on the length of a line.  Each line is passed to the consumer
// as-is, including its terminating newline (if any); this is tolerated by the
// JSON decoder, so no trimming is performed.  The slice passed to the consumer
// is only valid until the consumer returns.
func readLines(r io.Reader, consumer func([]byte) error) error {
	var (
		reader = bufio.NewReader(r)
		// line accumulates fragments of a line which does not fit within the
		// reader's internal buffer.  Its capacity is retained across lines.
		line []byte
	)
	//
	for {
		chunk, err := reader.ReadSlice('\n')
		// Line exceeds internal buffer, hence accumulate and keep reading.
		// Sentinel errors from bufio.Reader are never wrapped, so direct
		// comparison is sufficient (and cheaper than errors.Is).
		if err == bufio.ErrBufferFull {
			line = append(line, chunk...)
			continue
		} else if err != nil && err != io.EOF {
			return err
		}
		// Complete any line being accumulated.  Afterwards, line is reset to
		// the (possibly grown) buffer returned by append, so the largest buffer
		// seen is retained for subsequent long lines.  For short lines, chunk
		// is a view of the reader's buffer so no copy is needed.
		if len(line) > 0 {
			chunk = append(line, chunk...)
			line = chunk[:0]
		}
		// Skip empty tail at EOF (i.e. input ended with a newline)
		if len(chunk) > 0 {
			if err := consumer(chunk); err != nil {
				return err
			}
		}
		//
		if err == io.EOF {
			return nil
		}
	}
}
