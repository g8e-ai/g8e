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
