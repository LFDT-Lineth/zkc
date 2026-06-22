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
package vm

import (
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/checkpoint"
	"github.com/LFDT-Lineth/zkc/pkg/zkc/vm/internal/word"
)

// CheckPoint represents a captured state of an executing machine, such that
// execution can be continued later from this position (sometimes also known as
// a "continuation").  As such, the checkpoint must include all information
// necessary to allow execution to continue.  However, to reduce the size of a
// checkpoint, certain optimsiations of this information are permitted.  As
// such, implementations can manage this information in different ways (e.g.
// using compression, read-access lists, page-access lists, etc).
//
// For example suppose that a machine, upon reaching the checkpoint, has 1GB of
// some RAM allocated.  A full checkpoint would include all of this data and,
// therefore, be at least 1GB in size.  Suppose, however, that executing the
// machine from this point onwards only requires accessing 1MB of data from that
// RAM.  Then, in fact, a valid implementation might only store that 1MB of
// information (e.g. to reduce network transport costs, or proof size, etc).
//
// Finally, to support more aggressive optimisation, checkpoints have a notion
// of their "validity window".   To understand this, consider a refinement of
// the example above.  Suppose there are M execution steps remaining from the
// checkpoint until the machine terminates, but that executing these M steps now
// requires the full 1GB of RAM.  However, we now suppose that executing the
// next N steps requires accessing only 1MB of that RAM. Then, the checkpoint
// may only store that 1MB of data if it is marked as being valid for (upto) N
// steps of execution.  The purpose of this is to allow a program's execution to
// be broken up into multiple checkpoints, such that each checkpoint only stores
// the data it requires for executing its portion of the overall execution.
type CheckPoint[W word.Word[W]] = checkpoint.CheckPoint[W]
