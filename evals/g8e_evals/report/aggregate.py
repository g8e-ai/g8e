# Copyright (c) 2026 Lateralus Labs, LLC.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from typing import List, Dict, Any
from g8e_evals.harness import RowResult, Aggregate, BindingType

def aggregate_results(suite: str, results: List[RowResult]) -> Aggregate:
    total = len(results)
    if total == 0:
        return Aggregate(suite, 0.0, 0, 0, 0.0, 0.0)
        
    passed = sum(1 for r in results if r.score.passed)
    pass_rate = (passed / total) * 100.0
    
    bound = sum(1 for r in results if r.response.binding == BindingType.RECEIPT_BOUND)
    coverage = (bound / total) * 100.0
    
    verified = sum(1 for r in results if r.response.receipt_verified)
    # Verification % is of the bound ones, or of total?
    # Plan §8 says "receipt verification %" - let's do of total for honesty.
    verification_pct = (verified / total) * 100.0
    
    return Aggregate(
        suite=suite,
        pass_rate=pass_rate,
        total_tasks=total,
        passed_tasks=passed,
        receipt_coverage_pct=coverage,
        receipt_verification_pct=verification_pct
    )
