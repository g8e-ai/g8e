// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package constants

// TruncatedOutputFormat is the format string used when large command output is
// head/tail truncated before storage. %d is the number of bytes skipped.
const TruncatedOutputFormat = "%s\n\n[TRUNCATED: %d bytes skipped]\n\n%s"
