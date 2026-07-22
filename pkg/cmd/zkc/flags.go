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
	"fmt"
	"os"
	"strconv"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/spf13/cobra"
)

// VerboseLevel captures how much diagnostic output the zkc tooling produces.
// The levels are ordered, so higher levels are supersets of lower ones.
type VerboseLevel int

const (
	// VERBOSE_NONE is the default level: no extra output, only warnings and
	// errors are logged.
	VERBOSE_NONE VerboseLevel = iota
	// VERBOSE_INFO enables ordinary informational logging.
	VERBOSE_INFO
	// VERBOSE_DEBUG enables debug logging, notably the machine execution steps
	// reported during execution.
	VERBOSE_DEBUG
	// VERBOSE_PRINTF enables full output, additionally retaining and emitting
	// printf statements.
	VERBOSE_PRINTF
)

// GetVerboseLevel derives the verbosity from the repeatable "verbose" (-v)
// flag: absent selects NONE, -v selects INFO, -vv selects DEBUG, and -vvv
// selects PRINTF.
func GetVerboseLevel(cmd *cobra.Command) VerboseLevel {
	n, err := cmd.Flags().GetCount("verbose")
	if err != nil {
		fmt.Println(err)
		os.Exit(4)
	}
	//
	var verboseLevel VerboseLevel
	switch {
	case n <= 0:
		verboseLevel = VERBOSE_NONE
	case n == 1:
		verboseLevel = VERBOSE_INFO
	case n == 2:
		verboseLevel = VERBOSE_DEBUG
	case n == 3:
		verboseLevel = VERBOSE_PRINTF
	default:
		fmt.Printf("error: invalid --verbose %d (expected 0, 1, 2 or 3)\n", n)
		os.Exit(2)
	}

	return verboseLevel
}

// FlagChecks provides some additional feature over the base flags package used
// in Cobra.  Specifically, it allows to ensure certain flags are only used in
// conjunction with others or, conversely, that certain flags cannot be used in
// conjunction with others.
type FlagChecks struct {
	requires []util.Pair[string, string]
	excludes []util.Pair[string, string]
}

// Require asserts that a given flag requires another flag to be set.
func (p *FlagChecks) Require(flag, required string) {
	p.requires = append(p.requires, util.Pair[string, string]{Left: flag, Right: required})
}

// Exclude asserts that two given flags cannot be used together.
func (p *FlagChecks) Exclude(flag1, flag2 string) {
	p.excludes = append(p.excludes, util.Pair[string, string]{Left: flag1, Right: flag2})
}

func checkFlags(cmd *cobra.Command, checks FlagChecks) {
	// Check requires
	for _, check := range checks.requires {
		first := cmd.Flags().Changed(check.Left)

		second := cmd.Flags().Changed(check.Right)
		if first && !second {
			fmt.Printf("error: \"--%s\" requires \"--%s\"\n", check.Left, check.Right)
			os.Exit(1)
		}
	}
	// Check excludes
	for _, check := range checks.excludes {
		first := cmd.Flags().Changed(check.Left)

		second := cmd.Flags().Changed(check.Right)
		if first && second {
			fmt.Printf("error: \"--%s\" and \"--%s\" cannot be used together\n", check.Left, check.Right)
			os.Exit(1)
		}
	}
	// Some sanity check on some flag values:
	// Note: range checks require static tables of size at least 2.
	// no more the case once https://github.com/LFDT-Lineth/zkc/issues/1910 implemented
	if cmd.Flags().Changed("max-static-depth") {
		maxStaticDepth := GetUint(cmd, "max-static-depth")
		if maxStaticDepth <= 2 {
			fmt.Printf("error: \"--%s\" must be greater than 2\n", "max-static-depth")
			os.Exit(1)
		}
	}
}

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
