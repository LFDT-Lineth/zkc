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

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	log "github.com/sirupsen/logrus"
)

// Closeable captures the set of objects which can be closed.
type Closeable interface {
	// Handle returns an identifier to be associated with this object in the
	// event of an error.
	Handle() string
	// Close releases all resources associated with this object.
	Close() error
}

// RegisterExitHandler returns a function which, in turn, calls the given
// handler.  The intention of the handler is to do some final clean up before
// termination, and it is intended to be called exactly once.  The returned
// function is safe to call repeatedly and concurrently but, regardless, the
// handler is still called exactly once.  Typically, the returned function might
// be deferred around some computation which could panic, such that e.g.
// resources are cleaned up despite this.  The handler is also called when the
// process:
//
//   - exits through log.Exit() or log.Fatal().  Such exits bypass deferred
//     functions and, hence, commands in this package should not call os.Exit()
//     directly (else the handler will not be triggered).
//
//   - is interrupted (e.g. via ctrl-c) or asked to terminate. After cleanup,
//     the handler attempts to re-raise the signal. Other signal subscribers
//     may still intercept it.
func RegisterExitHandler(handler func() error) func() {
	var (
		clear = sync.OnceFunc(func() {
			if err := handler(); err != nil {
				log.Warn(err)
			}
		})
		// Chanel on which to receive signals
		signals = make(chan os.Signal, 1)
	)
	// Register exit handler to be called on log.Exit().
	log.RegisterExitHandler(clear)
	// Register handler for signals which would terminate the process.
	registerSignalHandler(signals, os.Interrupt, syscall.SIGTERM)
	// Start signal handler
	go func() {
		sig := <-signals
		// Stop this subscription before cleanup so it won't intercept another signal.
		signal.Stop(signals)
		// Clear all resources
		clear()
		// Raise signal again (now we've cleared our handler).
		raiseSignal(sig)
	}()
	//
	return clear
}

// Register a signal handler to be called when the given signals are triggered
// (unless those signals are explicitly blocked).
func registerSignalHandler(handler chan os.Signal, signals ...os.Signal) {
	for _, sig := range signals {
		if !signal.Ignored(sig) {
			signal.Notify(handler, sig)
		}
	}
}

// Re-raise given signal on this process.
func raiseSignal(sig os.Signal) {
	if p, err := os.FindProcess(os.Getpid()); err == nil {
		_ = p.Signal(sig) == nil
	}
}
