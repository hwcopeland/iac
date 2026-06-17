#!/usr/bin/env python3
"""JARVIS cluster-side agent runner — entrypoint for one tracked k8s Job.

Charter roadmap #2 (agent control-plane). This is the cluster-side
generalization of ``jarvis_delegate_mcp.delegate``: instead of spawning
``claude -p`` as a detached in-pod subprocess that hogs the voice loop,
each long-running task is its OWN k8s Job whose lifecycle/status IS the
run registry (see runner/job-template.yaml). This script is that Job's
process: it runs ONE task and exits.

Source-of-truth split (the key architectural decision):
  * k8s Job ``.status`` (Active/Succeeded/Failed) is the AUTHORITATIVE run
    status — exact, free, RBAC-gated, TTL-reaped. list_runs/run_status read
    it. This script does NOT try to maintain status; it just exits 0/non-0
    so the Job condition reflects reality.
  * mem0 user_id="runs" scope is the SECONDARY store: the human-readable
    start fact + result blurb that the Sonos completion-announce loop
    narrates and that "what did that run find?" recall queries hit later.

Reuses the jarvis-edge image verbatim (claude CLI + MCP servers + the
creds-seeding initContainer pattern). No new base image.

Env contract (set by launch_run on the Job spec):
  RUNNER_RUN_ID     required — the run id, e.g. "r17181234abcd"
  RUNNER_TASK       required — the task text for ``claude -p``
  RUNNER_MODE       read | apply  (default read; read-only tool scope)
  RUNNER_BUDGET_USD default 3 — claude --max-budget-usd cap
  RUNNER_WORKDIR    default ~/iac — claude working/--add-dir directory
  RUNNER_ISSUE      optional GitHub issue number (apply mode only)
  JARVIS_MEM_SCOPE  default "runs" — mem0 scope this run writes to
  MEM0_URL          default http://jarvis-mem0.ai.svc.cluster.local:8800
  MEM0_HTTP_TIMEOUT default 20
  RUNNER_MCP_CONFIG default /app/runner/runner_mcp_config.json
  RESULT_ANNOTATION_FILE optional path to write the result blurb to (the
                    Job's downward-API / sidecar can surface it; defaults
                    to /tmp/jarvis-run-result.txt)

Exit code: 0 on claude success, non-zero on claude failure — so the Job
``.status`` is truthful and the announce loop can say "finished" vs
"failed". mem0 write failures NEVER change the exit code (status is k8s's
job; mem0 is best-effort narration).

Stdlib only. Safe to import (no side effects at import time).
"""
from __future__ import annotations

import json
import os
import shlex
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request

# ── Config from env ──────────────────────────────────────────────────────
_MEM0_URL = os.environ.get(
    "MEM0_URL", "http://jarvis-mem0.ai.svc.cluster.local:8800"
).rstrip("/")
_MEM0_TIMEOUT = float(os.environ.get("MEM0_HTTP_TIMEOUT", "20"))
_MEM_SCOPE = os.environ.get("JARVIS_MEM_SCOPE", "runs").strip() or "runs"
_DEFAULT_MCP_CONFIG = "/app/runner/runner_mcp_config.json"
_DEFAULT_RESULT_FILE = "/tmp/jarvis-run-result.txt"
_JARVIS_BRANCH = "jarvis"
# Cap how much claude output we store / narrate — Sonos blurbs are short
# and mem0 extraction is lossy on long blobs.
_RESULT_BLURB_CHARS = 600


def _claude_bin() -> str:
    return shutil.which("claude") or os.path.expanduser("~/.local/bin/claude")


# ── mem0 best-effort writes (never raise into the run lifecycle) ──────────
def _mem0_add(text: str) -> bool:
    """POST /add {text, user_id} to the mem0 'runs' scope. Best-effort:
    returns True/False, never raises. Matches jarvis_mem0_mcp's contract."""
    text = (text or "").strip()
    if not text:
        return False
    payload = json.dumps({"text": text, "user_id": _MEM_SCOPE}).encode()
    req = urllib.request.Request(
        f"{_MEM0_URL}/add",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=_MEM0_TIMEOUT) as r:
            r.read()
        return True
    except (urllib.error.URLError, OSError, ValueError) as exc:
        # mem0 is narration-only; a failure here must NOT fail the run.
        print(f"[runner] mem0 /add failed (non-fatal): {exc}", file=sys.stderr)
        return False


# ── claude argv (mode-scoped, mirrors jarvis_delegate_mcp._build_cmd) ─────
def _jarvis_apply_system_prompt(issue: str | None) -> str:
    """Apply-mode policy: jarvis branch, never main, never close the issue.
    Kept in lockstep with jarvis_delegate_mcp._jarvis_system_prompt."""
    if issue:
        tail = (
            f"5. Comment on issue #{issue} with a short summary of what "
            f"changed, the verification you ran, and any caveats: "
            f"`gh issue comment {issue} -b \"...\"`. Do NOT close the "
            f"issue — the owner reviews and closes."
        )
    else:
        tail = (
            "5. Mention the changes in your final response so the owner "
            "knows what to review."
        )
    return (
        "\n\n## You're a JARVIS cluster run — branch + issue rules\n"
        "This task came from JARVIS (the owner's voice assistant) and runs "
        "as an autonomous cluster Job. All work on the owner's iac repos "
        "MUST live on the `jarvis` branch — NEVER commit to or push "
        "`main`. Required workflow:\n"
        "1. Confirm you're on the `jarvis` branch. If not: `git fetch "
        "origin && git checkout jarvis || git checkout -b jarvis "
        "origin/main`.\n"
        "2. Make the changes on `jarvis`.\n"
        "3. Commit (one logical commit per acceptance criterion; include "
        "`Co-Authored-By: JARVIS run <jarvis@local>`).\n"
        "4. Push: `git push -u origin jarvis`.\n"
        f"{tail}\n"
        "NEVER: push to `main`, merge PRs yourself, close issues without "
        "owner review, force-push `jarvis`."
    )


def _build_argv(task: str, mode: str, wd: str, budget: str,
                mcp_config: str, issue: str | None) -> list[str]:
    """Build the ``claude -p`` argv. Read mode = the SAME read-only
    allowlist as the delegate's 'diagnose'; apply mode = acceptEdits +
    jarvis-branch workflow. Returns argv (no shell) for argv-safe exec."""
    claude = _claude_bin()
    argv = [claude, "-p", task, "--model", "sonnet",
            "--add-dir", wd, "--max-budget-usd", str(budget)]
    if mcp_config and os.path.exists(mcp_config):
        argv += ["--mcp-config", mcp_config]

    if mode == "apply":
        argv += ["--permission-mode", "acceptEdits"]
        argv += ["--append-system-prompt", _jarvis_apply_system_prompt(issue)]
    else:  # read (default): read-only allowlist, mirrors delegate diagnose
        ro = ("Read Grep Glob WebFetch "
              "Bash(kubectl get*) Bash(kubectl describe*) Bash(kubectl logs*) "
              "Bash(kubectl top*) Bash(kubectl version*) Bash(kubectl explain*) "
              "Bash(git log*) Bash(git status*) Bash(git diff*) Bash(git show*) "
              "Bash(ls*) Bash(cat*) Bash(grep*) Bash(rg*) Bash(find*) Bash(curl*)")
        no = ("Edit Write NotebookEdit "
              "Bash(kubectl apply*) Bash(kubectl delete*) Bash(kubectl scale*) "
              "Bash(kubectl patch*) Bash(kubectl edit*) Bash(kubectl cordon*) "
              "Bash(kubectl drain*) Bash(kubectl rollout*) Bash(rm*) "
              "Bash(git push*) Bash(git commit*) Bash(helm*) Bash(flux*)")
        argv += ["--permission-mode", "bypassPermissions",
                 "--allowed-tools", ro,
                 "--disallowed-tools", no]
    return argv


def _apply_branch_prelude(wd: str, issue: str | None) -> None:
    """Apply mode only: check out the jarvis branch (and post a starting
    issue comment) before claude runs — mirrors the delegate's prelude.
    Best-effort; never aborts the run."""
    steps = [
        "git fetch origin --quiet 2>/dev/null || true",
        f"(git checkout {_JARVIS_BRANCH} 2>/dev/null || "
        f"git checkout -b {_JARVIS_BRANCH} origin/main 2>/dev/null) || true",
        f"git pull --rebase origin {_JARVIS_BRANCH} --quiet 2>/dev/null || true",
    ]
    if issue:
        start_msg = (f"JARVIS cluster run picking this up on the "
                     f"`{_JARVIS_BRANCH}` branch.")
        steps.append(
            f"gh issue comment {issue} -b {shlex.quote(start_msg)} "
            f">/dev/null 2>&1 || true"
        )
    cmd = f"cd {shlex.quote(wd)} && " + " && ".join(steps)
    try:
        subprocess.run(["bash", "-lc", cmd], check=False, timeout=120)
    except (OSError, subprocess.SubprocessError) as exc:
        print(f"[runner] branch prelude failed (non-fatal): {exc}",
              file=sys.stderr)


def _write_result_file(path: str, blurb: str) -> None:
    try:
        with open(path, "w") as f:
            f.write(blurb)
    except OSError as exc:
        print(f"[runner] result file write failed (non-fatal): {exc}",
              file=sys.stderr)


def _short_task(task: str, n: int = 80) -> str:
    t = " ".join((task or "").split())
    return t if len(t) <= n else t[: n - 1] + "…"


def main() -> int:
    run_id = (os.environ.get("RUNNER_RUN_ID") or "").strip()
    task = os.environ.get("RUNNER_TASK") or ""
    mode = (os.environ.get("RUNNER_MODE") or "read").strip().lower()
    if mode not in ("read", "apply"):
        mode = "read"
    budget = os.environ.get("RUNNER_BUDGET_USD", "3").strip() or "3"
    wd = os.path.expanduser(os.environ.get("RUNNER_WORKDIR", "~/iac"))
    issue = (os.environ.get("RUNNER_ISSUE") or "").strip() or None
    mcp_config = os.environ.get("RUNNER_MCP_CONFIG", _DEFAULT_MCP_CONFIG)
    result_file = os.environ.get("RESULT_ANNOTATION_FILE", _DEFAULT_RESULT_FILE)

    if not run_id:
        print("[runner] FATAL: RUNNER_RUN_ID is required", file=sys.stderr)
        return 2
    if not task.strip():
        print("[runner] FATAL: RUNNER_TASK is required", file=sys.stderr)
        return 2

    short = _short_task(task)
    print(f"[runner] run {run_id} mode={mode} budget=${budget} wd={wd}",
          file=sys.stderr)

    # (a) start fact → mem0 "runs" scope (best-effort narration)
    _mem0_add(f"run {run_id} started ({mode}): {short}")

    # apply-mode branch prelude (no-op for read)
    if mode == "apply":
        _apply_branch_prelude(wd, issue)

    argv = _build_argv(task, mode, wd, budget, mcp_config, issue)

    # (b)(c) run claude headless, capture stdout. HOME is /tmp so the
    # initContainer-seeded ~/.claude/.credentials.json resolves (subscription
    # OAuth). Strip ANTHROPIC_API_KEY so we stay in subscription mode.
    env = dict(os.environ)
    env.pop("ANTHROPIC_API_KEY", None)
    env["JARVIS_MEM_SCOPE"] = _MEM_SCOPE  # the runner's claude inherits "runs"

    out_chunks: list[str] = []
    rc = 1
    try:
        os.chdir(wd)
    except OSError:
        pass  # claude --add-dir still scopes it; cwd is convenience only
    try:
        proc = subprocess.Popen(
            argv, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            stdin=subprocess.DEVNULL, env=env, text=True, bufsize=1,
        )
        assert proc.stdout is not None
        for line in proc.stdout:
            sys.stdout.write(line)  # stream to Job logs live
            sys.stdout.flush()
            out_chunks.append(line)
        rc = proc.wait()
    except (OSError, subprocess.SubprocessError) as exc:
        print(f"[runner] claude spawn failed: {exc}", file=sys.stderr)
        rc = 127

    output = "".join(out_chunks).strip()
    blurb = output[-_RESULT_BLURB_CHARS:] if output else "(no output)"
    ok = rc == 0
    status_word = "ok" if ok else "error"

    # (d) result fact → mem0 + result file for the announce loop
    _mem0_add(f"run {run_id} finished ({status_word}): {blurb}")
    _write_result_file(result_file, blurb)

    print(f"[runner] run {run_id} finished ({status_word}) rc={rc}",
          file=sys.stderr)
    # (e) exit code mirrors claude so Job .status is truthful
    return 0 if ok else (rc if rc else 1)


if __name__ == "__main__":
    sys.exit(main())
