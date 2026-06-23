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
	"fmt"
	"os"
	"slices"
	"sort"

	"github.com/LFDT-Lineth/zkc/pkg/trace/lt"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/hash"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/set"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/bls12_377"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf251"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/gf8209"
	"github.com/LFDT-Lineth/zkc/pkg/util/field/koalabear"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
	"github.com/spf13/cobra"
)

// traceDiffCmd represents the trace command for manipulating traces.
var traceDiffCmd = &cobra.Command{
	Use:   "diff [flags] trace_file trace_file",
	Short: "Show differences between two trace files.",
	Long: `Reports differences between two trace files,
	which is useful when the trace files are expected to be identical.`,
	Run: func(cmd *cobra.Command, args []string) {
		runFieldAgnosticCmd(cmd, args, traceDiffCmds)
	},
}

// Available instances
var traceDiffCmds = []FieldAgnosticCmd{
	{field.GF_251, runTraceDiffCmd[gf251.Element]},
	{field.GF_8209, runTraceDiffCmd[gf8209.Element]},
	{field.KOALABEAR_16, runTraceDiffCmd[koalabear.Element]},
	{field.BLS12_377, runTraceDiffCmd[bls12_377.Element]},
}

func runTraceDiffCmd[F field.Element[F]](cmd *cobra.Command, args []string) {
	if len(args) != 2 {
		fmt.Println(cmd.UsageString())
		os.Exit(1)
	}
	// Extract trace file to diagnose
	filename1 := args[0]
	filename2 := args[1]
	// Read trace files
	tracefile1 := ReadTraceFile(filename1)
	tracefile2 := ReadTraceFile(filename2)
	// Extract column names
	t1names, t1cols := extractColumnNames(tracefile1.RawModules())
	t2names, t2cols := extractColumnNames(tracefile2.RawModules())
	// Sanity check
	if !slices.Equal(t1names, t2names) {
		var common set.SortedSet[string]
		//
		common.InsertSorted(&t1names)
		common.Intersect(&t2names)
		//
		reportExtraColumns(filename1, t1names, common)
		reportExtraColumns(filename2, t2names, common)
		t1cols = filterCommonColumns(t1cols, common)
		t2cols = filterCommonColumns(t2cols, common)
	}
	//
	errors := parallelDiff(t1cols, t2cols)
	// report any differences
	for _, err := range errors {
		fmt.Println(err)
	}
}

func extractColumnNames(modules []lt.Module[word.BigEndian]) (set.SortedSet[string], []RawColumn) {
	var (
		names   set.SortedSet[string]
		columns []RawColumn
	)
	//
	for _, ith := range modules {
		for _, jth := range ith.Columns {
			name := fmt.Sprintf("%s.%s", ith.Name(), jth.Name())
			names.Insert(name)
			columns = append(columns, lt.NewColumn(name, jth.MutData()))
		}
	}
	//
	return names, columns
}

func reportExtraColumns(name string, columns []string, common set.SortedSet[string]) {
	for _, c := range columns {
		if !common.Contains(c) {
			fmt.Printf("column %s only in trace %s\n", c, name)
		}
	}
}

func filterCommonColumns(columns []RawColumn, common set.SortedSet[string],
) []RawColumn {
	//
	var ncolumns []RawColumn
	//
	for _, c := range columns {
		if common.Contains(c.Name()) {
			ncolumns = append(ncolumns, c)
		}
	}
	//
	return ncolumns
}

func parallelDiff(columns1 []RawColumn, columns2 []RawColumn) []error {
	errors := make([]error, 0)
	ncols := len(columns1)
	// Look through all STAMP columns searching for 0s at the end.
	c := make(chan []error, ncols)
	// Dispatch go-routines
	for i := 0; i < ncols; i++ {
		go func(i int) {
			c <- diffColumns(i, columns1, columns2)
		}(i)
	}
	// Bring back together
	for i := uint(0); i < uint(ncols); i++ {
		// Read packaged result from channel
		res := <-c
		// Record any differences
		errors = append(errors, res...)
	}
	//
	return errors
}

func diffColumns(index int, columns1 []RawColumn, columns2 []RawColumn) []error {
	errors := make([]error, 0)
	name := columns1[index].Name()
	data1 := columns1[index].Data()
	data2 := findColumn(name, columns2).Data()
	// Sanity check
	if data2 == nil {
		return errors
	}
	//
	set1 := summarise(data1)
	set2 := summarise(data2)
	// Determine set of unique values
	vals := identify_vals(set1, set2)
	// Print differences
	for _, val := range vals {
		count1, ok1 := set1.Get(val)
		count2, ok2 := set2.Get(val)
		//
		if ok1 != ok2 || count1 != count2 {
			err := fmt.Errorf("column %s, element %s occurs %d or %d times", name, val.String(), count1, count2)
			errors = append(errors, err)
		}
	}
	//
	return errors
}

func identify_vals(lhs hash.Map[word.BigEndian, uint], rhs hash.Map[word.BigEndian, uint]) []word.BigEndian {
	seen := hash.NewSet[word.BigEndian](0)
	vals := make([]word.BigEndian, 0)
	// lhs
	for iter := lhs.KeyValues(); iter.HasNext(); {
		ith := iter.Next()
		val, count := ith.Split()
		//
		if !seen.Contains(val) && count > 0 {
			vals = append(vals, val)
			seen.Insert(val)
		}
	}
	// rhs
	for iter := rhs.KeyValues(); iter.HasNext(); {
		ith := iter.Next()
		val, count := ith.Split()
		//
		if !seen.Contains(val) && count > 0 {
			vals = append(vals, val)
			seen.Insert(val)
		}
	}
	// Sort items so the output is easier to understand
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].Cmp(vals[j]) < 0
	})
	//
	return vals
}

func summarise(data array.Array[word.BigEndian]) hash.Map[word.BigEndian, uint] {
	summary := *hash.NewMap[word.BigEndian, uint](data.Len())
	//
	for i := uint(0); i < data.Len(); i++ {
		ith := data.Get(i)
		if v, ok := summary.Get(ith); ok {
			summary.Insert(ith, v+1)
		} else {
			summary.Insert(ith, 1)
		}
	}
	//
	return summary
}

func findColumn(name string, columns []RawColumn) *RawColumn {
	for _, c := range columns {
		if c.Name() == name {
			return &c
		}
	}
	//
	fmt.Printf("WARNING: missing column %s", name)
	//
	return nil
}

func init() {
	traceCmd.AddCommand(traceDiffCmd)
}
