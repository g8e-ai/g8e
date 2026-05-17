import asyncio
import json
import os
from pathlib import Path
from datetime import datetime
from typing import Optional

import click
from rich.console import Console

from g8e_evals.harness import RowResult, BindingType, SUTConfig, LLMRoleConfig
from g8e_evals.sut.answer_only import AnswerOnlySUT
from g8e_evals.benchmarks.ifeval.loader import IFEvalLoader
from g8e_evals.benchmarks.ifeval.verifier import IFEvalVerifier
from g8e_evals.receipts.collector import ReceiptCollector
from g8e_evals.receipts.verify import verify_receipt_signature
from g8e_evals.report.aggregate import aggregate_results
from g8e_evals.report.cli_renderer import render_summary

console = Console()

@click.group()
def main():
    """g8e High-Fidelity AI Evaluation Harness"""
    pass

@main.command()
@click.option("--suite", type=click.Choice(["ifeval"]), required=True)
@click.option("--provider", type=click.Choice(["openai", "anthropic", "gemini", "ollama", "llamacpp"]), help="Primary LLM provider")
@click.option("--model", help="Primary model name (e.g., gpt-4o)")
@click.option("--assistant-provider", type=click.Choice(["openai", "anthropic", "gemini", "ollama", "llamacpp"]), help="Assistant LLM provider")
@click.option("--assistant-model", help="Assistant model name")
@click.option("--lite-provider", type=click.Choice(["openai", "anthropic", "gemini", "ollama", "llamacpp"]), help="Lite LLM provider")
@click.option("--lite-model", help="Lite model name")
@click.option("--mode", type=click.Choice(["receipt", "baseline"]), default="receipt")
@click.option("--operator-url", default="https://localhost:9000")
@click.option("--operator-id", envvar="OPERATOR_ID")
@click.option("--operator-session-id", envvar="OPERATOR_SESSION_ID")
@click.option("--state-root", default="test-state-root-v1")
@click.option("--output-dir", type=click.Path(path_type=Path), default=Path("reports"))
@click.option("--gold-set", type=click.Path(exists=True, path_type=Path))
@click.option("--limit", type=int, help="Limit number of tasks to run")
@click.option("--l2-key", help="L2 private key hex")
@click.option("--l2-key-id", help="L2 key ID")
@click.option("--primary-api-key", help="API key for the primary provider")
@click.option("--primary-endpoint", help="Endpoint URL for the primary provider")
@click.option("--assistant-api-key", help="API key for the assistant provider")
@click.option("--assistant-endpoint", help="Endpoint URL for the assistant provider")
@click.option("--lite-api-key", help="API key for the lite provider")
@click.option("--lite-endpoint", help="Endpoint URL for the lite provider")
def run(suite, model, provider, assistant_model, assistant_provider, lite_model, lite_provider, mode, operator_url, operator_id, operator_session_id, state_root, output_dir, gold_set, limit, l2_key, l2_key_id, primary_api_key, primary_endpoint, assistant_api_key, assistant_endpoint, lite_api_key, lite_endpoint):
    """Run a benchmark suite"""
    if mode == "receipt" and not (operator_id and operator_session_id):
        raise click.UsageError("operator-id and operator-session-id are required for receipt mode")

    config = SUTConfig(
        primary=LLMRoleConfig(provider=provider, model=model, api_key=primary_api_key, endpoint=primary_endpoint),
        assistant=LLMRoleConfig(provider=assistant_provider, model=assistant_model, api_key=assistant_api_key, endpoint=assistant_endpoint),
        lite=LLMRoleConfig(provider=lite_provider, model=lite_model, api_key=lite_api_key, endpoint=lite_endpoint),
        operator_url=operator_url,
        operator_id=operator_id,
        operator_session_id=operator_session_id,
        state_root=state_root,
        l2_private_key=l2_key,
        l2_key_id=l2_key_id,
        mode=mode
    )

    asyncio.run(_run_suite(suite, config, gold_set, output_dir, limit))

async def _run_suite(suite: str, config: SUTConfig, gold_set: Optional[Path], output_dir: Path, limit: Optional[int] = None):
    # 1. Load benchmark
    if suite == "ifeval":
        if not gold_set:
            gold_set = Path("gold_sets/ifeval/input_data.jsonl")
        loader = IFEvalLoader(gold_set)
        tasks = list(loader.load())
        if limit:
            tasks = tasks[:limit]
        verifier = IFEvalVerifier()
    
    # 2. Initialize SUT
    sut = AnswerOnlySUT(config)
    
    collector = ReceiptCollector(config.operator_url)
    
    # Load warden pub key for verification
    warden_pub_path = Path(os.environ.get("G8E_PKI_DIR", ".g8e/pki")) / "warden_pub.pem"
    warden_pub = ""
    if warden_pub_path.exists():
        warden_pub = warden_pub_path.read_text()

    results = []
    
    display_model = f"{config.primary.provider}:{config.primary.model}" if config.primary.provider and config.primary.model else (config.primary.model or "openai:gpt-4")
    console.print(f"[bold blue]Running {suite} with {display_model} in {config.mode} mode...[/bold blue]")
    
    # 3. Execution loop
    for task in tasks:
        # Create a descriptive summary for the task
        intent = ""
        if suite == "ifeval" and "instruction_id_list" in task.metadata:
            # Extract short names from instruction IDs (e.g., 'length:min_words' -> 'min_words')
            constraints = [id.split(":")[-1] for id in task.metadata["instruction_id_list"]]
            intent = f" [dim][{', '.join(constraints)}][/dim]"
        
        prompt_preview = task.prompt.replace("\n", " ")[:50]
        if len(task.prompt) > 50:
            prompt_preview += "..."
            
        console.print(f"  [cyan]{task.id:>4}[/cyan]: {prompt_preview}{intent} ...", end="")
        
        # Get answer
        response = await sut.get_answer(task)
        
        # Collect receipt if bound
        if response.binding == BindingType.RECEIPT_BOUND and response.transaction_id:
            receipt = await collector.collect_receipt(response.transaction_id)
            if receipt:
                response.receipt = receipt
                response.receipt_signature = receipt.get("signature")
                if warden_pub:
                    response.receipt_verified = verify_receipt_signature(receipt, warden_pub)
        
        # Score
        if suite == "ifeval":
            score = verifier.verify(
                task.id, 
                task.prompt, 
                response.answer, 
                task.metadata["instruction_id_list"],
                task.metadata["kwargs"]
            )
            
        res = RowResult(task=task, response=response, score=score)
        results.append(res)
        
        status_color = "green" if score.passed else "red"
        receipt_status = ""
        if response.binding == BindingType.RECEIPT_BOUND:
            if response.receipt_verified:
                receipt_status = " [cyan](verified)[/cyan]"
            else:
                receipt_status = " [yellow](unverified)[/yellow]"
                
        console.print(f" [{status_color}]Done[/{status_color}]{receipt_status}")

    # 4. Aggregate & Report
    agg = aggregate_results(suite, results)
    render_summary(agg)
    
    # 5. Save artifacts
    ts = datetime.now().strftime("%Y%m%d-%H%M%S")
    report_dir = output_dir / f"{suite}-{ts}"
    report_dir.mkdir(parents=True, exist_ok=True)
    
    def row_to_dict(r: RowResult):
        return {
            "task_id": r.task.id,
            "prompt": r.task.prompt,
            "answer": r.response.answer,
            "transaction_id": r.response.transaction_id,
            "receipt": r.response.receipt,
            "receipt_signature": r.response.receipt_signature,
            "receipt_verified": r.response.receipt_verified,
            "passed": r.score.passed,
            "details": r.score.details,
            "timestamp": r.timestamp.isoformat()
        }

    with open(report_dir / "results.jsonl", "w") as f:
        for r in results:
            f.write(json.dumps(row_to_dict(r)) + "\n")
            
    with open(report_dir / "summary.json", "w") as f:
        f.write(json.dumps(agg.__dict__, indent=2))
        
    console.print(f"\n[bold green]Report saved to {report_dir}[/bold green]")

@main.command()
@click.argument("report_dir", type=click.Path(exists=True, path_type=Path))
@click.option("--pki-dir", type=click.Path(exists=True, path_type=Path))
def verify_receipts(report_dir, pki_dir):
    """Re-verify all receipts in a report directory offline"""
    if not pki_dir:
        pki_dir = Path(os.environ.get("G8E_PKI_DIR", ".g8e/pki"))
    
    warden_pub_path = pki_dir / "warden_pub.pem"
    if not warden_pub_path.exists():
        console.print(f"[bold red]Error:[/bold red] Warden public key not found at {warden_pub_path}")
        return
        
    warden_pub = warden_pub_path.read_text()
    
    results_path = report_dir / "results.jsonl"
    if not results_path.exists():
        console.print(f"[bold red]Error:[/bold red] results.jsonl not found in {report_dir}")
        return
        
    console.print(f"[bold blue]Verifying receipts in {report_dir}...[/bold blue]")
    
    total = 0
    verified = 0
    failed = 0
    
    with open(results_path, "r") as f:
        for line in f:
            data = json.loads(line)
            receipt = data.get("receipt")
            if not receipt:
                continue
                
            total += 1
            if verify_receipt_signature(receipt, warden_pub):
                verified += 1
            else:
                failed += 1
                console.print(f"  [red]FAILED:[/red] Receipt for task {data.get('task_id')} (TX: {data.get('transaction_id')})")

    if total == 0:
        console.print("[yellow]No bound receipts found in report.[/yellow]")
    else:
        status = "green" if failed == 0 else "red"
        console.print(f"\n[{status}]Re-verification complete:[/{status}]")
        console.print(f"  Total receipts: {total}")
        console.print(f"  Verified: {verified}")
        console.print(f"  Failed: {failed}")
        
        if failed > 0:
            exit(1)

if __name__ == "__main__":
    main()
