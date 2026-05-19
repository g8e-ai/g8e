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
