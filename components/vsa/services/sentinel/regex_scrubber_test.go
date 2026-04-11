// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sentinel

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// RegexScrubber.Name
// ---------------------------------------------------------------------------

func TestRegexScrubber_Name_ReturnsConstructedName(t *testing.T) {
	s := &RegexScrubber{
		name:        "test_scrubber",
		pattern:     regexp.MustCompile(`secret`),
		replacement: "[REDACTED]",
	}
	assert.Equal(t, "test_scrubber", s.Name())
}

func TestRegexScrubber_Name_EmptyString(t *testing.T) {
	s := &RegexScrubber{
		name:        "",
		pattern:     regexp.MustCompile(`x`),
		replacement: "[X]",
	}
	assert.Equal(t, "", s.Name())
}

func TestRegexScrubber_Name_DoesNotScrub(t *testing.T) {
	s := &RegexScrubber{
		name:        "my_scrubber",
		pattern:     regexp.MustCompile(`secret`),
		replacement: "[REDACTED]",
	}
	// Name() must not modify any state — calling it is idempotent
	assert.Equal(t, s.Name(), s.Name())
}

// ---------------------------------------------------------------------------
// RegexScrubber.Scrub — verify the Name() method integrates with Scrub()
// ---------------------------------------------------------------------------

func TestRegexScrubber_Scrub_RemovesPattern(t *testing.T) {
	s := &RegexScrubber{
		name:        "api_key",
		pattern:     regexp.MustCompile(`secret\d+`),
		replacement: "[API_KEY]",
	}
	result := s.Scrub("my secret12345 is here")
	assert.Equal(t, "my [API_KEY] is here", result)
	assert.Equal(t, "api_key", s.Name())
}

// ---------------------------------------------------------------------------
// All built-in scrubbers have non-empty names (regression guard)
// ---------------------------------------------------------------------------

func TestSentinel_AllBuiltInScrubbers_HaveNonEmptyNames(t *testing.T) {
	cfg := DefaultSentinelConfig()
	s := NewSentinel(cfg, nil)

	for _, scrubber := range s.scrubbers {
		assert.NotEmpty(t, scrubber.Name(), "every built-in scrubber must have a non-empty name")
	}
}
