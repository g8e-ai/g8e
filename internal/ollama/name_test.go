// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ollama

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseName_AppliesDefaultsForOmittedParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Name
	}{
		{
			name:  "bare model gets default host namespace and tag",
			input: "llama3",
			want:  Name{Host: "registry.ollama.ai", Namespace: "library", Model: "llama3", Tag: "latest"},
		},
		{
			name:  "model with tag keeps the tag",
			input: "llama3:8b",
			want:  Name{Host: "registry.ollama.ai", Namespace: "library", Model: "llama3", Tag: "8b"},
		},
		{
			name:  "namespace and model default host and tag",
			input: "myorg/llama3",
			want:  Name{Host: "registry.ollama.ai", Namespace: "myorg", Model: "llama3", Tag: "latest"},
		},
		{
			name:  "namespace model and tag default only the host",
			input: "myorg/llama3:q4_K_M",
			want:  Name{Host: "registry.ollama.ai", Namespace: "myorg", Model: "llama3", Tag: "q4_K_M"},
		},
		{
			name:  "fully specified name keeps every part",
			input: "models.example.com/myorg/llama3:1b",
			want:  Name{Host: "models.example.com", Namespace: "myorg", Model: "llama3", Tag: "1b"},
		},
		{
			name:  "host port colon is not mistaken for a tag separator",
			input: "localhost:5000/myorg/llama3",
			want:  Name{Host: "localhost:5000", Namespace: "myorg", Model: "llama3", Tag: "latest"},
		},
		{
			name:  "host port with explicit tag",
			input: "localhost:5000/myorg/llama3:dev",
			want:  Name{Host: "localhost:5000", Namespace: "myorg", Model: "llama3", Tag: "dev"},
		},
		{
			name:  "url scheme is stripped from the host",
			input: "https://models.example.com/myorg/llama3:v1",
			want:  Name{Host: "models.example.com", Namespace: "myorg", Model: "llama3", Tag: "v1"},
		},
		{
			name:  "tag containing dots and dashes",
			input: "gpt-oss:20b-instruct.v2",
			want:  Name{Host: "registry.ollama.ai", Namespace: "library", Model: "gpt-oss", Tag: "20b-instruct.v2"},
		},
		{
			name:  "empty input yields only defaults",
			input: "",
			want:  Name{Host: "registry.ollama.ai", Namespace: "library", Model: "", Tag: "latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ParseName(tt.input))
		})
	}
}

func TestParseName_MarksEmptySegmentsMissingSoTheyAreNeverDefaulted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Name
	}{
		{
			name:  "trailing colon leaves the tag missing",
			input: "llama3:",
			want:  Name{Host: "registry.ollama.ai", Namespace: "library", Model: "llama3", Tag: missingPart},
		},
		{
			name:  "leading slash leaves the namespace missing",
			input: "/llama3",
			want:  Name{Host: "registry.ollama.ai", Namespace: missingPart, Model: "llama3", Tag: "latest"},
		},
		{
			name:  "trailing slash leaves the model missing",
			input: "myorg/",
			want:  Name{Host: "registry.ollama.ai", Namespace: "myorg", Model: missingPart, Tag: "latest"},
		},
		{
			name:  "leading colon leaves the model missing",
			input: ":8b",
			want:  Name{Host: "registry.ollama.ai", Namespace: "library", Model: missingPart, Tag: "8b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ParseName(tt.input)
			assert.Equal(t, tt.want, got)
			assert.False(t, got.IsFullyQualified(), "a name with a missing part must not be fully qualified")
		})
	}
}

func TestMerge_PrefersExplicitPartsAndNeverFillsTheModel(t *testing.T) {
	t.Parallel()

	defaults := Name{Host: "default-host", Namespace: "default-ns", Model: "default-model", Tag: "default-tag"}

	assert.Equal(t,
		Name{Host: "h", Namespace: "n", Model: "m", Tag: "t"},
		merge(Name{Host: "h", Namespace: "n", Model: "m", Tag: "t"}, defaults))
	assert.Equal(t,
		Name{Host: "default-host", Namespace: "default-ns", Model: "m", Tag: "default-tag"},
		merge(Name{Model: "m"}, defaults))
	assert.Equal(t,
		Name{Host: "h", Namespace: "default-ns", Model: "", Tag: "t"},
		merge(Name{Host: "h", Tag: "t"}, defaults),
		"the model is part of the identity and is never defaulted")
}

func TestName_IsFullyQualified(t *testing.T) {
	t.Parallel()

	valid := Name{Host: "registry.ollama.ai", Namespace: "library", Model: "llama3.1", Tag: "8b-instruct_q4"}
	assert.True(t, valid.IsFullyQualified())

	tests := []struct {
		name   string
		mutate func(*Name)
	}{
		{name: "empty host", mutate: func(n *Name) { n.Host = "" }},
		{name: "empty namespace", mutate: func(n *Name) { n.Namespace = "" }},
		{name: "empty model", mutate: func(n *Name) { n.Model = "" }},
		{name: "empty tag", mutate: func(n *Name) { n.Tag = "" }},
		{name: "host with slash", mutate: func(n *Name) { n.Host = "a/b" }},
		{name: "namespace with dot", mutate: func(n *Name) { n.Namespace = "my.org" }},
		{name: "model with colon", mutate: func(n *Name) { n.Model = "llama:3" }},
		{name: "tag with space", mutate: func(n *Name) { n.Tag = "8b instruct" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name := valid
			tt.mutate(&name)
			assert.False(t, name.IsFullyQualified())
		})
	}
}

func TestName_ManifestFilepath_JoinsPartsHostFirst(t *testing.T) {
	t.Parallel()

	got := ParseName("myorg/llama3:q4").ManifestFilepath()

	assert.Equal(t, filepath.Join("registry.ollama.ai", "myorg", "llama3", "q4"), got)
}

func TestIsValidPart_AppliesPerKindCharacterRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind partKind
		part string
		want bool
	}{
		{name: "host plain domain", kind: kindHost, part: "registry.ollama.ai", want: true},
		{name: "host with port", kind: kindHost, part: "localhost:11434", want: true},
		{name: "host starting with underscore", kind: kindHost, part: "_internal", want: true},
		{name: "host starting with dash", kind: kindHost, part: "-bad", want: false},
		{name: "host starting with dot", kind: kindHost, part: ".bad", want: false},
		{name: "host with at sign", kind: kindHost, part: "user@host", want: false},
		{name: "namespace with dash and underscore", kind: kindNamespace, part: "my-org_1", want: true},
		{name: "namespace with dot", kind: kindNamespace, part: "my.org", want: false},
		{name: "namespace with colon", kind: kindNamespace, part: "my:org", want: false},
		{name: "model with dot", kind: kindModel, part: "llama3.1", want: true},
		{name: "model with colon", kind: kindModel, part: "llama:3", want: false},
		{name: "tag with dot dash and underscore", kind: kindTag, part: "v1.0-rc_2", want: true},
		{name: "tag with colon", kind: kindTag, part: "a:b", want: false},
		{name: "non-ascii byte", kind: kindModel, part: "mödel", want: false},
		{name: "non-ascii first byte", kind: kindModel, part: "ömodel", want: false},
		{name: "empty part", kind: kindModel, part: "", want: false},
		{name: "missing sentinel", kind: kindModel, part: missingPart, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isValidPart(tt.kind, tt.part))
		})
	}
}

func TestIsValidLen_EnforcesPerKindMaximum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		kind   partKind
		length int
		want   bool
	}{
		{name: "host minimum", kind: kindHost, length: 1, want: true},
		{name: "host maximum", kind: kindHost, length: 350, want: true},
		{name: "host above maximum", kind: kindHost, length: 351, want: false},
		{name: "host empty", kind: kindHost, length: 0, want: false},
		{name: "namespace maximum", kind: kindNamespace, length: 80, want: true},
		{name: "namespace above maximum", kind: kindNamespace, length: 81, want: false},
		{name: "model maximum", kind: kindModel, length: 80, want: true},
		{name: "model above maximum", kind: kindModel, length: 81, want: false},
		{name: "tag maximum", kind: kindTag, length: 80, want: true},
		{name: "tag above maximum", kind: kindTag, length: 81, want: false},
		{name: "tag empty", kind: kindTag, length: 0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, isValidLen(tt.kind, strings.Repeat("a", tt.length)))
		})
	}
}

func TestIsAlphanumericOrUnderscore_ClassifiesAsciiBoundaries(t *testing.T) {
	t.Parallel()

	for _, c := range []byte{'a', 'z', 'A', 'Z', '0', '9', '_'} {
		assert.True(t, isAlphanumericOrUnderscore(c), string(c))
	}
	for _, c := range []byte{'-', '.', ':', '/', ' ', '@', '[', '`', '{', '!', 0xc3} {
		assert.False(t, isAlphanumericOrUnderscore(c), string(c))
	}
}

func TestCutLast_SplitsOnTheFinalSeparator(t *testing.T) {
	t.Parallel()

	before, after, ok := cutLast("a/b/c", "/")
	assert.Equal(t, "a/b", before)
	assert.Equal(t, "c", after)
	assert.True(t, ok)

	before, after, ok = cutLast("abc", "/")
	assert.Equal(t, "abc", before)
	assert.Empty(t, after)
	assert.False(t, ok)

	before, after, ok = cutLast("a::b", "::")
	assert.Equal(t, "a", before)
	assert.Equal(t, "b", after)
	assert.True(t, ok, "multi-byte separators advance past their full length")
}

func TestCutPromised_ReplacesEmptySidesOfAPresentSeparatorWithMissingMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantBefore string
		wantAfter  string
		wantOK     bool
	}{
		{name: "both sides present", input: "a/b", wantBefore: "a", wantAfter: "b", wantOK: true},
		{name: "empty before", input: "/b", wantBefore: missingPart, wantAfter: "b", wantOK: true},
		{name: "empty after", input: "a/", wantBefore: "a", wantAfter: missingPart, wantOK: true},
		{name: "separator alone", input: "/", wantBefore: missingPart, wantAfter: missingPart, wantOK: true},
		{name: "separator absent is not promised", input: "abc", wantBefore: "abc", wantAfter: "", wantOK: false},
		{name: "empty input is not promised", input: "", wantBefore: "", wantAfter: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			before, after, ok := cutPromised(tt.input, "/")
			assert.Equal(t, tt.wantBefore, before)
			assert.Equal(t, tt.wantAfter, after)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}
