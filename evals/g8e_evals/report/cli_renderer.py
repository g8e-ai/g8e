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

from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from g8e_evals.harness import Aggregate

def render_summary(agg: Aggregate):
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
    
    if agg.receipt_coverage_pct == 0:
        console.print(Panel(
            "[bold yellow]HINT:[/bold yellow] Receipt coverage is 0.00%. To enable receipts:\n"
            "1. Start the platform: [cyan]./g8e platform start[/cyan] "
            "(auto-provisions the sandbox bootstrap superuser)\n"
            "2. Authenticate the local CLI: [cyan]./g8e login[/cyan] "
            "(no flags needed in sandbox; switch users with [cyan]--email <addr>[/cyan])\n"
            "3. Re-run without [cyan]--mode baseline[/cyan]. Credentials are auto-loaded "
            "from [cyan]~/.g8e/credentials[/cyan].",
            border_style="yellow"
        ))
    elif agg.receipt_verification_pct < 100.0:
        console.print(Panel("[bold red]WARNING:[/bold red] Some receipts failed verification!", border_style="red"))
    elif agg.receipt_verification_pct == 100.0:
        console.print(Panel("[bold green]SUCCESS:[/bold green] All receipts verified!", border_style="green"))
