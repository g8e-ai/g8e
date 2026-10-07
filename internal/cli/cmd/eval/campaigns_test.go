// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package eval

import (
	evalv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/eval/v1"
)

func threeModelRegistry() []*evalv1.ModelVariant {
	return []*evalv1.ModelVariant{
		{VariantId: "gemma4-e4b", ServedModelTag: "gemma4:e4b", ModelDigest: repeatTestHex('a'), ProviderClass: "ollama", ParameterCount: 4_000_000_000, ModelFamily: "Gemma"},
		{VariantId: "qwen3-4b", ServedModelTag: "qwen3:4b", ModelDigest: repeatTestHex('b'), ProviderClass: "ollama", ParameterCount: 4_000_000_000, ModelFamily: "Qwen"},
		{VariantId: "llama3-8b", ServedModelTag: "llama3:8b", ModelDigest: repeatTestHex('c'), ProviderClass: "ollama", ParameterCount: 8_000_000_000, ModelFamily: "Llama"},
	}
}
