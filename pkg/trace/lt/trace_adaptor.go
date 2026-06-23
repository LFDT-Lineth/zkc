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
package lt

import (
	"github.com/LFDT-Lineth/zkc/pkg/trace"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/array"
	"github.com/LFDT-Lineth/zkc/pkg/util/collection/pool"
	"github.com/LFDT-Lineth/zkc/pkg/util/field"
	"github.com/LFDT-Lineth/zkc/pkg/util/word"
)

// FromRawTrace constructs a TraceFile from an instance of trace.Trace[F].  This
// is always safe since the trace instance contains values of some field which
// will always git within a word.BigEndian.
func FromRawTrace[F field.Element[F]](metadata []byte, trace trace.Trace[F]) TraceFile {
	var (
		heap      = pool.NewLocalHeap[word.BigEndian]()
		builder   = array.NewDynamicBuilder(heap)
		trModules = trace.Modules().Collect()
		ltModules = make([]Module[word.BigEndian], len(trModules))
	)
	//
	for i, m := range trModules {
		ltModules[i] = reconstructLtTraceModule(m, &builder)
	}
	//
	return NewTraceFile(metadata, *heap, ltModules)
}

func reconstructLtTraceModule[F field.Element[F]](module trace.Module[F], builder array.Builder[word.BigEndian],
) Module[word.BigEndian] {
	var (
		columns []Column[word.BigEndian] = make([]Column[word.BigEndian], module.Width())
	)
	// iterate columns
	for i := range module.Width() {
		columns[i] = reconstructLtTraceColumn[F](module.Column(i), builder)
	}
	// construct new module
	return NewModule(module.Name(), columns)
}

func reconstructLtTraceColumn[F field.Element[F]](col trace.Column[F], builder array.Builder[word.BigEndian],
) Column[word.BigEndian] {
	//
	var (
		data = col.Data()
		// construct new column
		ncol = builder.NewArray(data.Len(), data.BitWidth())
	)
	// populate column
	for i := range data.Len() {
		var (
			v = data.Get(i)
			w word.BigEndian
		)
		// Copy over bytes
		w = w.SetBytes(v.Bytes())
		// Done
		ncol.Set(i, w)
	}
	//
	return NewColumn(col.Name(), ncol)
}
