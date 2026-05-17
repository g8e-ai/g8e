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
            "1. Ensure the Operator is running: [cyan]./g8e platform start[/cyan]\n"
            "2. Login to get a session: [cyan]./g8e login[/cyan]\n"
            "3. Run without [cyan]--mode baseline[/cyan] and provide [cyan]--operator-session-id[/cyan]",
            border_style="yellow"
        ))
    elif agg.receipt_verification_pct < 100.0:
        console.print(Panel("[bold red]WARNING:[/bold red] Some receipts failed verification!", border_style="red"))
    elif agg.receipt_verification_pct == 100.0:
        console.print(Panel("[bold green]SUCCESS:[/bold green] All receipts verified!", border_style="green"))
