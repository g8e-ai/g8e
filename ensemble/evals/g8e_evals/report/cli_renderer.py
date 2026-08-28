# Copyright (c) 2026 Lateralus Labs, LLC.
# Use of this source code is governed by the Business Source License
# included in the LICENSE file.
#
# As of the Change Date listed in the LICENSE file, this software is
# released under the Apache License, Version 2.0.

from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from g8e_evals.harness import Aggregate

def render_summary(agg: Aggregate, mode: str | None = None):
    console = Console()

    table = Table(title=f"Benchmark Results: {agg.suite}")
    table.add_column("Metric", style="cyan")
    table.add_column("Value", style="magenta")

    table.add_row("Total Tasks", str(agg.total_tasks))
    table.add_row("Passed Tasks", str(agg.passed_tasks))
    table.add_row("Pass Rate", f"{agg.pass_rate:.2f}%")
    table.add_row("Receipt Coverage", f"{agg.receipt_coverage_pct:.2f}%")
    table.add_row("Receipt Verification", f"{agg.receipt_verification_pct:.2f}%")

    console.print(table)

    if agg.receipt_coverage_pct == 0 and mode != "receipt":
        console.print(Panel(
            "[bold yellow]HINT:[/bold yellow] Receipt coverage is 0.00%. To enable receipts:\n"
            "1. Start the gateway: [cyan]./g8e gw start[/cyan]\n"
            "2. Enroll the local CLI: [cyan]./g8e auth enroll user[/cyan]\n"
            "3. Refresh an expired session if needed: [cyan]./g8e auth refresh[/cyan]\n"
            "4. Re-run without [cyan]--mode baseline[/cyan]. Authentication is loaded "
            "through [cyan]./g8e auth context[/cyan].",
            border_style="yellow"
        ))
    elif agg.receipt_coverage_pct > 0 and agg.receipt_verification_pct < 100.0:
        console.print(Panel("[bold red]WARNING:[/bold red] Some receipts failed verification!", border_style="red"))
    elif agg.receipt_coverage_pct > 0 and agg.receipt_verification_pct == 100.0:
        console.print(Panel("[bold green]SUCCESS:[/bold green] All receipts verified!", border_style="green"))
