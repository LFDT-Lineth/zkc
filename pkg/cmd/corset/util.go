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
package corset

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/rtrace"
	"github.com/LFDT-Lineth/zkc/pkg/rtrace/json"
	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/file"
	"github.com/spf13/cobra"
)

// GetFlag gets an expected flag, or panic if an error arises.
func GetFlag(cmd *cobra.Command, flag string) bool {
	r, err := cmd.Flags().GetBool(flag)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	return r
}

// GetInt gets an expectedsigned integer, or panic if an error arises.
func GetInt(cmd *cobra.Command, flag string) int {
	r, err := cmd.Flags().GetInt(flag)
	if err != nil {
		fmt.Println(err)
		os.Exit(3)
	}

	return r
}

// GetUint gets an expected unsigned integer, or panic if an error arises.
func GetUint(cmd *cobra.Command, flag string) uint {
	r, err := cmd.Flags().GetUint(flag)
	if err != nil {
		fmt.Println(err)
		os.Exit(4)
	}

	return r
}

// GetString gets an expected string, or panic if an error arises.
func GetString(cmd *cobra.Command, flag string) string {
	r, err := cmd.Flags().GetString(flag)
	if err != nil {
		fmt.Println(err)
		os.Exit(4)
	}

	return r
}

// GetStringArray gets an expected string array, or panic if an error arises.
func GetStringArray(cmd *cobra.Command, flag string) []string {
	r, err := cmd.Flags().GetStringArray(flag)
	if err != nil {
		fmt.Println(err)
		os.Exit(4)
	}

	return r
}

// GetIntArray gets an expected int array, or panic if an error arises.
func GetIntArray(cmd *cobra.Command, flag string) []int {
	tmp, err := cmd.Flags().GetStringArray(flag)
	if err != nil {
		fmt.Println(err)
		os.Exit(4)
	}
	//
	r := make([]int, len(tmp))
	//
	for i, str := range tmp {
		ith, err := strconv.ParseInt(str, 16, 8)
		// Error check
		if err != nil {
			panic(err.Error())
		}
		//
		r[i] = int(ith)
	}
	//
	return r
}

// nolint
func writeBatchedTracesFile[F field.Element[F]](filename string, traces ...rtrace.Trace[F]) {
	var buf bytes.Buffer
	// Check file extension
	if len(traces) == 1 {
		writeTraceFile(filename, traces[0])
		return
	}
	// Always write JSON in batched mode
	for _, trace := range traces {
		js := json.ToJsonString(trace)
		buf.WriteString(js)
		buf.WriteString("\n")
	}
	// Write file
	if err := os.WriteFile(filename, buf.Bytes(), 0644); err != nil {
		// Handle error
		fmt.Println(err)
		os.Exit(4)
	}
}

// Write a given trace file to disk
// nolint
func writeTraceFile[F field.Element[F]](filename string, tracefile rtrace.Trace[F]) {
	var err error
	// Check file extension
	ext := path.Ext(filename)
	//
	switch ext {
	case ".json":
		js := json.ToJsonString(tracefile)
		//
		if err = os.WriteFile(filename, []byte(js), 0644); err == nil {
			return
		}
	default:
		err = fmt.Errorf("unknown trace file format: %s", ext)
	}
	// Handle error
	fmt.Println(err)
	os.Exit(4)
}

// ReadTraceFile reads a JSON trace file and parses it into an array of raw
// columns.
func ReadTraceFile[F field.Element[F]](filename string) rtrace.Trace[F] {
	var (
		stats     = util.NewPerfStats()
		tracefile rtrace.Trace[F]
	)
	// Read data file
	filename, data, err := file.ReadAndUncompress(filename)
	// Check success
	if err == nil {
		// Check file extension
		ext := path.Ext(filename)
		//
		switch ext {
		case ".json":
			tracefile, err = json.FromBytes[F](data)
		default:
			err = fmt.Errorf("unknown trace file format: %s", ext)
		}
	}
	//
	stats.Log("Reading trace file")
	//
	if err != nil {
		// Handle error
		fmt.Println(err)
		os.Exit(2)
	}
	//
	return tracefile
}

// ReadBatchedTraceFile reads a file containing zero or more traces expressed as
// JSON, where each trace is on a separate line.
func ReadBatchedTraceFile[F field.Element[F]](filename string) []rtrace.Trace[F] {
	var (
		stats    = util.NewPerfStats()
		lines, _ = file.ReadInputFileAsLines(filename)
		traces   = make([]rtrace.Trace[F], 0)
	)
	// Read constraints line by line
	for i, line := range lines {
		// Parse input line as JSON
		if line != "" && !strings.HasPrefix(line, ";;") {
			trace, err := json.FromBytes[F]([]byte(line))
			if err != nil {
				msg := fmt.Sprintf("%s:%d: %s", filename, i+1, err)
				panic(msg)
			}

			traces = append(traces, trace)
		}
	}
	//
	stats.Log("Reading trace file")
	//
	return traces
}
