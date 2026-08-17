// Copyright Consensys Software Inc.
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
package zkc

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/cmd/corset"
	"github.com/LFDT-Lineth/zkc/pkg/cmd/zkc/gogen"
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/constraints"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var executeCmd = &cobra.Command{
	Use:     "execute [flags] input.json file1.zkc file2.zkc ...",
	Short:   "Execute a zkc program.",
	Long:    `Execute a zkc program to produce a set of outputs a from given a set of inputs.`,
	Aliases: []string{"exec"},
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, executeCmds)
	},
}

// Available instances
var executeCmds = []FieldAgnosticCmd{
	{field.GF_251, runExecuteCmd[gf251.Element]},
	{field.GF_8209, runExecuteCmd[gf8209.Element]},
	{field.KOALABEAR_16, runExecuteCmd[koalabear.Element]},
	{field.BLS12_377, runExecuteCmd[bls12_377.Element]},
}

// Permitted flag combinations
var executeFlags FlagChecks

func runExecuteCmd[F field.Element[F]](cmd *cobra.Command, args []string, field field.Config) {
	var (
		errors []error
		build  = GetBuildConfig[F](cmd, field)
		// outputFile file for tracep
		outputFile = GetString(cmd, "output")
		// check constraints
		check = GetFlag(cmd, "check")
		// checkpoint spec: when set (as "FUNCTION:INTERVAL"), checkpoint every
		// INTERVAL-th call to FUNCTION and execute in fast mode, printing
		// checkpoints.
		checkpoint = GetString(cmd, "checkpoint")
		// resume mode: the input file holds a checkpoint (hex) to resume from,
		// rather than an initial set of JSON inputs.  Execution then continues to
		// completion via the fast bytecode interpreter.
		resume = GetFlag(cmd, "resume")
		// simple equivalence
		tracing = !build.fastMode
		//
		trace   trace.Trace[F]
		input   map[string][]byte
		outputs map[string][]byte
	)
	// Configure tracing
	traceConfig := constraints.DEFAULT_TRACE_CONFIG.
		WithPadding(build.padding).
		WithBatchSize(GetUint(cmd, "batch"))
	// Sanity permitted flag combinations
	checkFlags(cmd, executeFlags)
	// Build artifacts (compiles source files or loads a prebuilt binary).
	_, binfile := Build[F](build, args[1:]...)
	// =====================================================
	// Trace / Execute
	// =====================================================
	if resume {
		// Resume execution from the checkpoint held (in hex) in the input file,
		// running the (unmodified) program to completion in fast mode.
		outputs, errors = resumeFromCheckPoint(binfile.ExecutionProgram(), args[0])
	} else {
		// Parse an filter input file
		input = vm.FilterInputs(binfile.RawProgram(), ParseInputFile(args[0]))
		// decide what is happening
		if checkpoint != "" {
			// Checkpoint the function named in the spec (periodically with "f:N", or
			// once with "f@N") and execute in fast mode, writing the resulting
			// checkpoints to the output file (with -o) or to stdout otherwise.
			outputs, errors = executeWithCheckPoint(binfile.ExecutionProgram(), checkpoint, outputFile, input)
		} else if tracing {
			outputs, _, trace, errors = binfile.Trace(input, traceConfig)
		} else if build.gogen {
			// Execute via native Go generated from the word machine.
			outputs, errors = executeWithGogen(binfile.RawProgram(), input)
		} else {
			outputs, errors = binfile.Execute(input)
		}
	}
	// =====================================================
	// Generate output
	// =====================================================
	// Write outputs
	for name, bytes := range outputs {
		fmt.Printf("%s = 0x%s\n", name, hex.EncodeToString(bytes))
	}
	// write out trace (if requested)
	if tracing && outputFile != "" {
		// Construct trace file
		ltf := lt.FromRawTrace(nil, trace)
		// Write out trace file
		WriteTraceFile(outputFile, ltf)
	}
	// =====================================================
	// Check Constraints
	// =====================================================
	if check && trace != nil {
		// NOTE: check ==> tracing
		checkConstraints(binfile, trace, traceConfig)
	}
	// =====================================================
	// Report Execution Failures
	// =====================================================
	if len(errors) > 0 {
		// Log errors
		for _, e := range errors {
			log.Error(fmt.Sprintf("%s", e))
		}
		//
		os.Exit(4)
	}
}

func checkConstraints[F field.Element[F]](binfile *constraints.BinaryFile[F], tr trace.Trace[F],
	cfg constraints.TraceConfig) {
	//
	var checkConfig corset.CheckConfig
	// Set sensible defaults (for now)
	checkConfig.Report = true
	checkConfig.ReportCellWidth = 32
	checkConfig.ReportTitleWidth = 40
	checkConfig.ReportPadding = 2
	checkConfig.ReportLimbs = true
	checkConfig.ReportComputed = true
	checkConfig.AnsiEscapes = true
	// Construct limbs map
	mapping := binfile.LimbsMap()
	// Run the check
	if failures := binfile.Check(tr, cfg); len(failures) > 0 {
		corset.ReportFailures("AIR", failures, tr, mapping, checkConfig)
	}
}

// ============================================================================
// Misc
// ============================================================================

//nolint:errcheck
func init() {
	rootCmd.AddCommand(executeCmd)
	executeCmd.Flags().StringP("output", "o", "", "specify output file for writing trace")
	executeCmd.Flags().BoolP("check", "c", false, "check generated trace against constraints")
	executeCmd.PersistentFlags().UintP("batch", "b", 1024, "specify batch size for constraint checking")
	executeCmd.Flags().String("checkpoint", "",
		"checkpoint a function: \"f:N\" on every Nth call to f, or \"f@N\" once after N calls to f")
	executeCmd.Flags().Bool("resume", false,
		"resume from a checkpoint: the input file is a hex checkpoint (rather than JSON inputs) to execute to completion")
	// Checkpointing is only supported in fast mode.
	executeFlags.Require("checkpoint", "fast")
	// Resuming from a checkpoint (currently) is only supported in fast mode.
	// Eventually, this restriction should be lifted.
	executeFlags.Require("resume", "fast")
	// Gogen only supports fast mode (for now).
	executeFlags.Require("gogen", "fast")
	// Gogen does not support checkpointing (for now)
	executeFlags.Exclude("checkpoint", "gogen")
	// Fast mode cannot be used to check constraints (i.e. because it does not
	// generate a trace).
	executeFlags.Exclude("check", "fast")
}

// executeWithCheckPoint executes the word machine via the fast bytecode
// interpreter, having first switched every call to the checkpointed function
// into a checkpointing call.  The spec is "FUNCTION:INTERVAL" or
// "FUNCTION@INTERVAL" (see parseCheckPointSpec); checkpoints are written -- one
// hex string per line -- to the given output file (or to stdout when outputFile
// is empty).
func executeWithCheckPoint[W vm.Word[W]](program vm.Program[W], spec, outputFile string,
	input map[string][]byte) (map[string][]byte, []error) {
	//
	fn, clk, err := parseCheckPointSpec(spec)
	if err != nil {
		return nil, []error{err}
	}
	//
	interp, closer, err := newCheckPointInterpreter(program, fn, clk, outputFile)
	if err != nil {
		return nil, []error{err}
	}
	//
	defer closer()
	//
	output, _, errs := vm.BootAndExecute(interp, input, 131072)
	//
	return output, errs
}

// parseCheckPointSpec parses a --checkpoint specification, returning the
// function name, the (positive) interval, and whether the checkpoint is
// one-shot.  Two forms are accepted:
//
//	FUNCTION:INTERVAL  checkpoint on every INTERVAL-th invocation of FUNCTION
//	FUNCTION@INTERVAL  checkpoint once, after INTERVAL invocations of FUNCTION
//
// The interval is the suffix after the final ':' or '@' (whichever comes
// later), so function names that themselves contain ':' are handled correctly.
func parseCheckPointSpec(spec string) (string, util.Counter, error) {
	var (
		colon = strings.LastIndex(spec, ":")
		at    = strings.LastIndex(spec, "@")
	)
	// The later separator delimits the interval; '@' selects the one-shot form.
	if at > colon {
		fn, nstr := spec[:at], spec[at+1:]
		n, err := strconv.ParseUint(nstr, 10, 64)
		//
		return fn, util.NewCounterOnce(n), err
	} else {
		fn, istr := spec[:colon], spec[colon+1:]
		n, err := strconv.ParseUint(istr, 10, 64)
		//
		return fn, util.NewCounter(n), err
	}
}

// resumeFromCheckPoint resumes execution from the checkpoint held (as a hex
// string) in inputFile.  The program is rebuilt in fast mode, the
// interpreter's state is restored from the checkpoint, and execution continues
// to completion.  Registering a breakpoint does not alter instruction offsets
// (see Program.BreakPoint), so checkpoints are written in the plain program's
// coordinates (see executeWithCheckPoint) and resume directly against it -- no
// breakpoint needs to be registered here.
func resumeFromCheckPoint[W vm.Word[W]](prog vm.Program[W], inputFile string) (map[string][]byte, []error) {
	//
	var (
		stats  = util.NewPerfStats()
		interp = vm.NewBytecodeInterpreter(prog)
	)
	// Decode the checkpoint and restore the interpreter's state from it.
	cp, err := readCheckPoint[W](inputFile)
	if err != nil {
		return nil, []error{err}
	}
	//
	interp.Restore(cp)
	// Continue execution to completion.
	if nsteps, err := vm.ExecuteAll(interp, 131072); err != nil {
		return nil, []error{err}
	} else {
		// Log stats
		stats.Log(fmt.Sprintf("Machine execution (%d steps)", nsteps))
	}
	//
	return vm.EncodeOutputs(interp), nil
}

// newCheckPointInterpreter lowers the word machine to the fast (128-bit)
// bytecode form (mirroring BinaryFile.Execute), registers a breakpoint at fn's
// entry, and returns an interpreter whose breakpointer writes a hex checkpoint
// line to the output file (or stdout) on every interval-th entry of fn -- or,
// when once is set, exactly once after the interval-th entry.  The returned
// closer must be invoked once execution has completed.
//
// Registering a breakpoint only sets a modifier bit on the target instruction
// (see Program.BreakPoint), which is width-preserving; the resulting bytecode
// layout is therefore identical to the unmodified program.  Captured
// checkpoints thus share the original program's coordinates and resume directly
// against it (see resumeFromCheckPoint).
func newCheckPointInterpreter[W vm.Word[W]](p vm.Program[W], fn string, clk util.Counter, outputFile string,
) (*vm.Interpreter[W], func(), error) {
	//
	var (
		// Checkpoints are written here: the output file (with -o) or stdout.
		out    io.Writer = os.Stdout
		closer           = func() {}
	)
	// When an output file is given, write checkpoints there instead of stdout.
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return nil, nil, err
		}
		//
		out = f
		closer = func() {
			if err = f.Close(); err != nil {
				panic(err)
			}
		}
	}
	// Locate the function whose calls are to be checkpointed.
	fid, ok := p.HasModule(fn)
	if !ok {
		return nil, nil, fmt.Errorf("unknown function \"%s\"", fn)
	}
	// Register a breakpoint at fn's entry and build an interpreter for the
	// result, so the breakpointer fires each time fn is entered.
	p = p.BreakPoint(fid, vm.ProgramPoint{Macro: 0, Micro: 0})
	interp := vm.NewBytecodeInterpreter(p)
	// Write a checkpoint as a hex string, one per line.  The counter governs how
	// frequently this actually fires: it triggers every interval entries of fn.
	emit := func(_ uint32) {
		// Only record once every interval-th invocation of fn.
		if !clk.Tick() {
			return
		}
		//
		bytes, err := interp.CheckPoint().MarshalBinary()
		if err != nil {
			log.Errorf("encoding checkpoint: %s", err)
			return
		}
		//
		if _, err := fmt.Fprintf(out, "0x%s\n", hex.EncodeToString(bytes)); err != nil {
			log.Errorf("encoding checkpoint: %s", err)
			return
		}
	}
	//
	interp.BreakPointer(emit)
	//
	return interp, closer, nil
}

// readCheckPoint reads a single checkpoint, encoded as a hex string (optionally
// 0x-prefixed) in the given file, and decodes it.
func readCheckPoint[W vm.Word[W]](file string) (vm.CheckPoint[W], error) {
	var cp vm.CheckPoint[W]
	//
	data, err := os.ReadFile(file)
	if err != nil {
		return cp, err
	}
	// Tolerate surrounding whitespace and an optional 0x prefix.
	text := strings.TrimPrefix(strings.TrimSpace(string(data)), "0x")
	//
	raw, err := hex.DecodeString(text)
	if err != nil {
		return cp, err
	}
	//
	if err := cp.UnmarshalBinary(raw); err != nil {
		return cp, err
	}
	//
	return cp, nil
}

// executeWithGogen executes the word machine by generating native Go, compiling
// it, and running the resulting binary as a subprocess — the same path the
// gogen differential tests take, exposed here as the "--gogen" execution mode.
func executeWithGogen(program vm.Program[vm.Uint], input map[string][]byte) (map[string][]byte, []error) {
	var (
		stats = util.NewPerfStats()
	)
	// Generate native Go source for the word machine.
	src, err := vm.GenerateGo(program, vm.GoGenConfig{})
	if err != nil {
		return nil, []error{err}
	}
	// Compile the generated source to a temporary executable.
	prog, cleanup, err := gogen.Build(src)
	if err != nil {
		return nil, []error{err}
	}
	// Log result
	stats.Log(fmt.Sprintf("compiling binary %s", prog))
	//
	defer cleanup()
	// Run the compiled program, passing only the machine's declared inputs (the
	// generated harness rejects unknown keys).  Forward its debug/printf and
	// fail output to our stderr so those statements are surfaced to the user.
	outputs, errored, err := gogen.Run(prog, input, os.Stderr)
	//
	switch {
	case err != nil:
		return nil, []error{err}
	case errored:
		return nil, []error{fmt.Errorf("execution rejected (trace rejected)")}
	}
	//
	return outputs, nil
}
