# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from g8e_evals.arms import Arm
from g8e_evals.harness import Aggregate

def render_summary(agg: Aggregate, arm: Arm | None = None):
    console = Console()

    arm_label = arm.value if arm is not None else "unknown"
    table = Table(title=f"Benchmark Results: {agg.suite} [arm={arm_label}]")
    table.add_column("Metric", style="cyan")
    table.add_column("Value", style="magenta")

    table.add_row("Total Tasks", str(agg.total_tasks))
    table.add_row("Passed Tasks", str(agg.passed_tasks))
    table.add_row("Pass Rate", f"{agg.pass_rate:.2f}%")
    table.add_row("Receipt Coverage", f"{agg.receipt_coverage_pct:.2f}%")
    table.add_row("Receipt Verification", f"{agg.receipt_verification_pct:.2f}%")

    console.print(table)

    # The canonical receipt-binding arms (doctrine, consensus, notary) expect
    # nonzero receipt coverage. Ratify is a supported gateway posture but does
    # not have a standalone eval arm. Ungoverned arms (direct,
    # ensemble_ungoverned) do not bind receipts by design.
    receipt_binding_arm = arm is not None and arm.value in ("doctrine", "consensus", "notary")

    if agg.receipt_coverage_pct == 0 and not receipt_binding_arm:
        console.print(Panel(
            f"[bold yellow]NOTE:[/bold yellow] Receipt coverage is 0.00% for arm [cyan]{arm_label}[/cyan]. "
            "This arm does not bind receipts by design. To measure receipt coverage, "
            "re-run with [cyan]--arm doctrine[/cyan], [cyan]--arm consensus[/cyan], or [cyan]--arm notary[/cyan].",
            border_style="yellow"
        ))
    elif agg.receipt_coverage_pct == 0 and receipt_binding_arm:
        console.print(Panel(
            "[bold yellow]HINT:[/bold yellow] Receipt coverage is 0.00% for a receipt-binding arm. To enable receipts:\n"
            "1. Start the gateway: [cyan]./g8e gw start[/cyan]\n"
            "2. Enroll the local CLI: [cyan]./g8e auth enroll user[/cyan]\n"
            "3. Refresh an expired session if needed: [cyan]./g8e auth refresh[/cyan]\n"
            "4. Re-run with a governed arm. Authentication is loaded "
            "through [cyan]./g8e auth context[/cyan].",
            border_style="yellow"
        ))
    elif agg.receipt_coverage_pct > 0 and agg.receipt_verification_pct < 100.0:
        console.print(Panel("[bold red]WARNING:[/bold red] Some receipts failed verification!", border_style="red"))
    elif agg.receipt_coverage_pct > 0 and agg.receipt_verification_pct == 100.0:
        console.print(Panel("[bold green]SUCCESS:[/bold green] All receipts verified!", border_style="green"))
