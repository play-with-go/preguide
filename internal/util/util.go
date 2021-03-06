// Copyright 2020 The play-with-go.dev Authors. All rights reserved.  Use of
// this source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package util

// knownErr is the sentinel error type used by check and raise. Values of this
// type are recovered in mainerr. See thd documentation for check for more
// details.
type KnownErr struct{ Err error }

// handleKnown is a convenience function used in a defer to recover from a
// knownErr. See the usage in mainerr.
func HandleKnown(err *error) {
	switch r := recover().(type) {
	case nil:
	case KnownErr:
		*err = r.Err
	default:
		panic(r)
	}
}
