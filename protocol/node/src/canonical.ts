// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

import { fromJson, toJson, type DescMessage, type MessageShape } from '@bufbuild/protobuf';

const canonicalJsonOptions = {
  useProtoFieldName: true,
};

export function serializeCanonical<Desc extends DescMessage>(
  schema: Desc,
  message: MessageShape<Desc>,
): string {
  const json = toJson(schema, message, canonicalJsonOptions);
  return JSON.stringify(json);
}

export function parseCanonical<Desc extends DescMessage>(
  schema: Desc,
  encoded: string,
  message: MessageShape<Desc>,
): MessageShape<Desc> {
  const parsed = fromJson(schema, JSON.parse(encoded), { ignoreUnknownFields: false });
  const canonical = serializeCanonical(schema, parsed);
  if (canonical !== encoded) {
    throw new Error('evaluation protocol: input is not canonical protojson');
  }
  Object.assign(message, parsed);
  return message;
}
