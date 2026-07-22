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
	"strings"

	"github.com/LFDT-Lineth/zkc/pkg/util"
	"github.com/spf13/cobra"
)

// VerboseLevel captures how much diagnostic output the zkc tooling produces.
// The levels are ordered, so higher levels are supersets of lower ones.
type VerboseLevel int

const (
	// VERBOSE_NONE disables extra diagnostic output (the default).
	VERBOSE_NONE VerboseLevel = iota
	// VERBOSE_INFO enables informational logging, notably the machine
	// execution steps reported during execution.
	VERBOSE_INFO
	// VERBOSE_DEBUG enables full debug output, additionally retaining and
	// emitting printf statements.
	VERBOSE_DEBUG
)

// GetVerboseLevel parses the "verbose-level" flag, exiting with an error on an
// unrecognised value.  Parsing is case-insensitive.
func GetVerboseLevel(cmd *cobra.Command) VerboseLevel {
	switch strings.ToUpper(strings.TrimSpace(GetString(cmd, "verbose-level"))) {
	case "NONE":
		return VERBOSE_NONE
	case "INFO":
		return VERBOSE_INFO
	case "DEBUG":
		return VERBOSE_DEBUG
	default:
		fmt.Printf("error: invalid --verbose-level %q (expected NONE, INFO or DEBUG)\n",
			GetString(cmd, "verbose-level"))
		os.Exit(2)
		return VERBOSE_NONE
	}
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
