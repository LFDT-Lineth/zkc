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
package util

import "github.com/LFDT-Lineth/zkc/pkg/util/option"

// Option provides a simple encoding for an optional value.  A key advantage
// over a pointer is that this serialises nicely.
type Option[T any] = option.Option[T]

// Some constructs an option which holds a value.
func Some[T any](val T) Option[T] {
	return option.Some(val)
}

// None constructs an option which doesn't hold a value.
func None[T any]() Option[T] {
	return option.None[T]()
}
