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
package file

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path"
	"strings"
)

// ReadAndUncompress reads a given file and, if its extension indicates it is
// compressed, then it decompresses it before returning the decompressed byte
// and the underlying filename (e.g. if original filename was "file.lt.gz" then
// it returns "file.lt").  Supported compression formats are "bz2" and "gz".
func ReadAndUncompress(filename string) (string, []byte, error) {
	var (
		ext    = path.Ext(filename)
		reader io.Reader
		err    error
		data   []byte
	)
	// Open file for reading
	file, err := os.Open(filename)
	// Sanity check for error
	if err != nil {
		return filename, nil, err
	}
	// Setup file close behaviour
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()
	//
	reader = file
	// check extension
	switch ext {
	case ".bz2":
		reader = bzip2.NewReader(reader)
		filename = strings.TrimSuffix(filename, ext)
	case ".gz":
		if reader, err = gzip.NewReader(reader); err != nil {
			return filename, nil, err
		}
		//
		filename = strings.TrimSuffix(filename, ext)
	}
	//
	data, err = io.ReadAll(reader)
	//
	return filename, data, err
}

// ReadInputFileAsLines reads an input file as a sequence of lines.  If the file
// does not exist, then ok==false is returned.
func ReadInputFileAsLines(filename string) (lines []string, ok bool) {
	file, err := os.Open(filename)
	// Check whether file exists
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, false
	} else if err != nil {
		panic(err)
	}
	// apply compression
	var reader io.Reader
	// check extension
	switch path.Ext(filename) {
	case ".bz2":
		reader = bzip2.NewReader(file)
	default:
		reader = file
	}
	//
	bufReader := bufio.NewReaderSize(reader, 1024*128)
	lines = make([]string, 0)
	// Read file line-by-line
	for {
		// Read the next line
		line := readLine(bufReader)
		// Check whether for EOF
		if line == nil {
			if err = file.Close(); err != nil {
				panic(err)
			}

			return lines, true
		}

		lines = append(lines, *line)
	}
}

// Read a single line
func readLine(reader *bufio.Reader) *string {
	var (
		bytes []byte
		bit   []byte
		err   error
	)
	//
	cont := true
	//
	for cont {
		bit, cont, err = reader.ReadLine()
		if err == io.EOF {
			return nil
		} else if err != nil {
			panic(err)
		}

		bytes = append(bytes, bit...)
	}
	// Convert to string
	str := string(bytes)
	// Done
	return &str
}
