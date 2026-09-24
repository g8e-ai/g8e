// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package ollama

import (
	"cmp"
	"path/filepath"
	"strings"
)

const (
	defaultHost      = "registry.ollama.ai"
	defaultNamespace = "library"
	defaultTag       = "latest"
	missingPart      = "!MISSING!"
)

type partKind int

const (
	kindHost partKind = iota
	kindNamespace
	kindModel
	kindTag
)

// Name is a structured representation of an Ollama model tag.
type Name struct {
	Host      string
	Namespace string
	Model     string
	Tag       string
}

// ParseName parses a served model tag and applies Ollama's default host,
// namespace, and tag when those parts are omitted.
func ParseName(s string) Name {
	return merge(parseNameBare(s), defaultName())
}

func defaultName() Name {
	return Name{
		Host:      defaultHost,
		Namespace: defaultNamespace,
		Tag:       defaultTag,
	}
}

func parseNameBare(s string) Name {
	var n Name

	if strings.LastIndex(s, ":") > strings.LastIndex(s, "/") {
		s, n.Tag, _ = cutPromised(s, ":")
	}

	s, model, promised := cutPromised(s, "/")
	if !promised {
		n.Model = s
		return n
	}
	n.Model = model

	s, namespace, promised := cutPromised(s, "/")
	if !promised {
		n.Namespace = s
		return n
	}
	n.Namespace = namespace

	scheme, host, ok := strings.Cut(s, "://")
	if ok {
		_ = scheme
	} else {
		host = scheme
	}
	n.Host = host

	return n
}

func merge(a, b Name) Name {
	a.Host = cmp.Or(a.Host, b.Host)
	a.Namespace = cmp.Or(a.Namespace, b.Namespace)
	a.Tag = cmp.Or(a.Tag, b.Tag)
	return a
}

// IsFullyQualified reports whether host, namespace, model, and tag are present
// and valid.
func (n Name) IsFullyQualified() bool {
	parts := []string{n.Host, n.Namespace, n.Model, n.Tag}
	for i, part := range parts {
		if !isValidPart(partKind(i), part) {
			return false
		}
	}
	return true
}

// ManifestFilepath returns the canonical manifest path relative to the models
// root in the form {host}/{namespace}/{model}/{tag}.
func (n Name) ManifestFilepath() string {
	return filepath.Join(n.Host, n.Namespace, n.Model, n.Tag)
}

func isValidLen(kind partKind, s string) bool {
	switch kind {
	case kindHost:
		return len(s) >= 1 && len(s) <= 350
	default:
		return len(s) >= 1 && len(s) <= 80
	}
}

func isValidPart(kind partKind, s string) bool {
	if !isValidLen(kind, s) {
		return false
	}
	for i := range s {
		if i == 0 {
			if !isAlphanumericOrUnderscore(s[i]) {
				return false
			}
			continue
		}
		switch s[i] {
		case '_', '-':
		case '.':
			if kind == kindNamespace {
				return false
			}
		case ':':
			if kind != kindHost {
				return false
			}
		default:
			if !isAlphanumericOrUnderscore(s[i]) {
				return false
			}
		}
	}
	return true
}

func isAlphanumericOrUnderscore(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
}

func cutLast(s, sep string) (before, after string, ok bool) {
	i := strings.LastIndex(s, sep)
	if i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

func cutPromised(s, sep string) (before, after string, ok bool) {
	before, after, ok = cutLast(s, sep)
	if !ok {
		return before, after, false
	}
	return cmp.Or(before, missingPart), cmp.Or(after, missingPart), true
}
