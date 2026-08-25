// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Business Source License 1.1 — see LICENSE for details.

const Rtf = {
    LOCALE:  'en',
    NUMERIC: 'always',
};

const Dtf = {
    LOCALE:    'en-CA',
    TIMEZONE:  'UTC',
    SUFFIX:    ' UTC',
};

const Relative = {
    IN_THE_PAST: 'in the past',
};

export const TimestampFormat = Object.freeze({
    RTF_LOCALE:      Rtf.LOCALE,
    RTF_NUMERIC:     Rtf.NUMERIC,
    DTF_LOCALE:      Dtf.LOCALE,
    DTF_TIMEZONE:    Dtf.TIMEZONE,
    DISPLAY_SUFFIX:  Dtf.SUFFIX,
    IN_THE_PAST:     Relative.IN_THE_PAST,
});
