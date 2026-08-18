// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

import "testing"

// BenchmarkActionTypeIsMutation benchmarks the IsMutation check for mutation types.
func BenchmarkActionTypeIsMutation_Mutation(b *testing.B) {
	a := ActionTypeFileEdit
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.IsMutation()
	}
}

// BenchmarkActionTypeIsMutation_NonMutation benchmarks the IsMutation check for non-mutation types.
func BenchmarkActionTypeIsMutation_NonMutation(b *testing.B) {
	a := ActionTypeFsRead
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.IsMutation()
	}
}
