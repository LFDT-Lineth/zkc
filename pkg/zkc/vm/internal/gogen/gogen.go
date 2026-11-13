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
	"encoding/binary"
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

// Binary interchange between a generated harness and its parent (i.e. Binary):
// the harness writes stdout as a sequence of packets, each a header of one byte
// (the packet kind) followed by a big endian uint32 (the payload length) and
// then the payload itself.  Checkpoints precede the (single, final) output.
const (
	packetHeaderSize = 5
	// packetCheckpoint carries a checkpoint as encoded by checkpoint.Marshal.
	packetCheckpoint byte = 0
	// packetOutput carries the output memories as a JSON object mapping each
	// memory to its hex string (as per hexString in the generated code).
	packetOutput byte = 1
)

// ErrMachineFailure is used to indicate when a gogen binary leads to a machine
// failure, as opposed to any other kind of internal error.
var ErrMachineFailure = errors.New("machine failure")

// Config controls the shape of the generated artefact.
type Config struct {
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
	// Json selects newline-delimited JSON (rather than the binary packets
	// described above) as the interchange format written to stdout by the
	// harness: checkpoints as {"$":"0x…"} and the output as a JSON object.
	// Since this dramatically reduces performance, and Binary only speaks the
	// binary format, it is only intended for debugging generated programs.
	Json bool
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
		// packetConsumer decodes each packet written by the harness, passing
		// checkpoints on to the checkpoint consumer and retaining the output.
		packetConsumer = func(kind byte, payload []byte) error {
			switch kind {
			case packetCheckpoint:
				var cp checkpoint.CheckPoint
				// Unmarshal aliases the payload, which is safe as readPackets
				// allocates a fresh payload for every packet.
				if err := cp.Unmarshal(payload); err != nil {
					return fmt.Errorf("decoding checkpoint: %w", err)
				}
				//
				nCheckpoints++
				// pass on checkpoint
				consumer(cp)
				//
				return nil
			case packetOutput:
				var err error
				//
				output, err = unmarshalOutput(payload)
				//
				return err
			default:
				return fmt.Errorf("decoding generated output: unknown packet kind %d", kind)
			}
		}
	)
	// Marshal input data into bytes
	if input, err = marshalInputs(in); err != nil {
		return nil, nCheckpoints, err
	} else if err = runRaw(p.path, input, packetConsumer, stderr); err != nil {
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

// unmarshalOutput decodes the payload of an output packet: a JSON object
// mapping each output memory to its hex string.
func unmarshalOutput(payload []byte) (map[string]bit.Array, error) {
	var raw map[string]string
	// Parse JSON
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("decoding generated output %q: %v", string(payload), err)
	}
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

func runRaw(path string, input []byte, consumer func(byte, []byte) error, stderr io.Writer) error {
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
	// Process output packets
	if err = readPackets(stdout, consumer); err != nil {
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

// readPackets splits r into packets (see packetHeaderSize) and passes the kind
// and payload of each one to the given consumer, stopping at the first error.
// A stream ending part way through a packet is an error.  Every payload is a
// fresh allocation (which the consumer may therefore retain), read directly
// from r without an intermediate copy.
func readPackets(r io.Reader, consumer func(byte, []byte) error) error {
	var (
		reader = bufio.NewReader(r)
		header [packetHeaderSize]byte
	)
	//
	for {
		// io.ReadFull reports io.EOF only when no bytes at all were read, i.e.
		// exactly at a packet boundary; a truncated header is ErrUnexpectedEOF.
		if _, err := io.ReadFull(reader, header[:]); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("reading packet header: %w", err)
		}
		//
		payload := make([]byte, binary.BigEndian.Uint32(header[1:]))
		//
		if _, err := io.ReadFull(reader, payload); err != nil {
			return fmt.Errorf("reading packet payload: %w", err)
		} else if err := consumer(header[0], payload); err != nil {
			return err
		}
	}
}
