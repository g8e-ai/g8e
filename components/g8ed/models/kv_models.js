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

import { G8eBaseModel, F, now } from './base.js';

export class KVSortedSetEntry extends G8eBaseModel {
    static fields = {
        score:  { type: F.number, required: true },
        member: { type: F.string, required: true },
    };
}

export class KVHash extends G8eBaseModel {
    static fields = {
        entries: { type: F.object, default: () => ({}) },
    };

    static fromRaw(raw) {
        if (!raw || typeof raw !== 'object') return new KVHash({ entries: {} });
        return KVHash.parse(raw);
    }

    get(field) {
        const val = this.entries[field];
        return val !== undefined ? val : null;
    }

    set(field, value) {
        this.entries[field] = value;
    }

    del(fields) {
        let count = 0;
        for (const f of fields) {
            if (f in this.entries) {
                delete this.entries[f];
                count++;
            }
        }
        return count;
    }

}

export class KVList extends G8eBaseModel {
    static fields = {
        items: { type: F.array, default: () => [] },
    };

    static fromRaw(raw) {
        if (!raw) return new KVList({ items: [] });
        return KVList.parse(raw);
    }
}

export class KVSet extends G8eBaseModel {
    static fields = {
        members: { type: F.array, default: () => [] },
    };

    static fromRaw(raw) {
        if (!raw) return new KVSet({ members: [] });
        return KVSet.parse(raw);
    }

    add(newMembers) {
        let added = 0;
        for (const m of newMembers) {
            if (!this.members.includes(m)) {
                this.members.push(m);
                added++;
            }
        }
        return added;
    }

    remove(toRemove) {
        const before = this.members.length;
        this.members = this.members.filter(m => !toRemove.includes(m));
        return before - this.members.length;
    }

}

export class KVSortedSet extends G8eBaseModel {
    static fields = {
        entries: { type: F.array, items: KVSortedSetEntry, default: () => [] },
    };

    static fromRaw(raw) {
        if (!raw) return new KVSortedSet({ entries: [] });
        return KVSortedSet.parse(raw);
    }

    upsert(score, member) {
        const idx = this.entries.findIndex(e => e.member === member);
        if (idx >= 0) {
            this.entries[idx].score = score;
            this.entries.sort((a, b) => a.score - b.score);
            return 0;
        }
        this.entries.push(new KVSortedSetEntry({ score, member }));
        this.entries.sort((a, b) => a.score - b.score);
        return 1;
    }

    remove(toRemove) {
        const before = this.entries.length;
        this.entries = this.entries.filter(e => !toRemove.includes(e.member));
        return before - this.entries.length;
    }

}

export class KVStreamEntry extends G8eBaseModel {
    static fields = {
        id:     { type: F.string, required: true },
        fields: { type: F.array, default: () => [] },
    };
}

export class KVStream extends G8eBaseModel {
    static fields = {
        entries: { type: F.array, items: KVStreamEntry, default: () => [] },
    };

    static fromRaw(raw) {
        if (!raw) return new KVStream({ entries: [] });
        return KVStream.parse(raw);
    }

    append(id, fields) {
        const entryId = id === '*' ? `${now().getTime()}-${this.entries.length}` : id;
        this.entries.push(new KVStreamEntry({ id: entryId, fields }));
        return entryId;
    }

    range(start, end) {
        return this.entries.filter(entry => {
            const afterStart = start === '-' || entry.id >= start;
            const beforeEnd = end === '+' || entry.id <= end;
            return afterStart && beforeEnd;
        });
    }

}
