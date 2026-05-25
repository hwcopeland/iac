"""Stdio MCP server: spawn Claude Code sub-agents from JARVIS's brain.

Mirrors the policy of ``_DelegateClaudeCodeTool`` (the fallback-agent
delegate) but is callable by the brain via the MCP boundary. Same rules:

  * mode='apply' → checks out the `jarvis` branch first, posts a
    starting comment on the GitHub issue (if given), appends a strong
    system prompt requiring jarvis-branch commits + push + summary
    comment + NEVER close-the-issue.
  * mode='diagnose' / 'plan' → no branch dance, read-only or plan-only.

The brain stays read-only — the sub-agent is the one with write power,
and only via this single narrow tool.

Spawn is detached (``start_new_session=True``) so the brain's MCP call
returns immediately. Output is captured to
``~/.openjarvis/sub_agent_logs/<job_id>.log`` so you can tail it.

Local/uncommitted, like the rest of JARVIS.
"""
from __future__ import annotations

import json
import os
import shlex
import shutil
import subprocess
import sys
import time
import traceback
from typing import Optional

_JARVIS_BRANCH = "jarvis"
_LOG_DIR = os.path.expanduser("~/.openjarvis/sub_agent_logs")


def _claude_bin() -> str:
    return shutil.which("claude") or os.path.expanduser("~/.local/bin/claude")


def _jarvis_system_prompt(issue: Optional[int]) -> str:
    """Same policy text the fallback agent uses — kept in sync with
    ``_DelegateClaudeCodeTool._jarvis_system_prompt`` in voice_daemon_cmd.py.
    """
    if issue:
        tail = (
            f"5. Comment on issue #{issue} with a short summary of what "
            f"changed, the verification you ran, and any caveats: "
            f"`gh issue comment {issue} -b \"...\"`. Do NOT close the "
            f"issue — the user reviews and closes."
        )
    else:
        tail = (
            "5. Mention the changes in your final response so the user "
            "knows what to review."
        )
    return (
        "\n\n## You're working on behalf of JARVIS — branch + issue rules\n"
        "This task came from JARVIS (the user's voice assistant). All "
        "work on the user's iac repos MUST live on the `jarvis` branch — "
        "NEVER commit to or push `main`. Required workflow:\n"
        "1. Confirm you're on the `jarvis` branch (the daemon already "
        "checked it out). If not: `git fetch origin && git checkout "
        "jarvis || git checkout -b jarvis origin/main`.\n"
        "2. Make the changes on `jarvis`.\n"
        "3. Commit (one logical commit per acceptance criterion; include "
        "`Co-Authored-By: JARVIS sub-agent <jarvis@local>`).\n"
        "4. Push: `git push -u origin jarvis`.\n"
        f"{tail}\n"
        "NEVER: push to `main`, merge PRs yourself, close issues without "
        "user review, force-push `jarvis`."
    )


def _build_cmd(task: str, mode: str, wd: str,
               issue: Optional[int]) -> str:
    """Compose the shell command line that runs claude in the right mode
    with prelude (branch checkout + optional issue comment)."""
    claude = _claude_bin()

    extra_sys = ""
    if mode == "apply":
        perm = "--permission-mode acceptEdits"
        extra_sys = _jarvis_system_prompt(issue)
    elif mode == "plan":
        perm = "--permission-mode plan"
    else:  # diagnose: read-only allowlist mirrors the brain itself
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
        perm = (f"--permission-mode bypassPermissions "
                f"--allowed-tools {shlex.quote(ro)} "
                f"--disallowed-tools {shlex.quote(no)}")

    prelude = ""
    if mode == "apply":
        br = _JARVIS_BRANCH
        steps = [
            "git fetch origin --quiet 2>/dev/null || true",
            f"(git checkout {br} 2>/dev/null || "
            f"git checkout -b {br} origin/main 2>/dev/null) || true",
            f"git pull --rebase origin {br} --quiet 2>/dev/null || true",
        ]
        if issue:
            start_msg = f"JARVIS sub-agent picking this up on the `{br}` branch."
            steps.append(
                f"gh issue comment {issue} -b {shlex.quote(start_msg)} "
                f">/dev/null 2>&1 || true"
            )
        prelude = " && ".join(steps) + " && "

    sys_arg = (f"--append-system-prompt {shlex.quote(extra_sys)} "
               if extra_sys else "")
    return (
        f"cd {shlex.quote(wd)} && "
        f"{prelude}"
        f"env -u ANTHROPIC_API_KEY {shlex.quote(claude)} -p {shlex.quote(task)} "
        f"{perm} {sys_arg}--add-dir {shlex.quote(wd)} "
        f"--max-budget-usd 2 --model sonnet"
    )


def delegate(task: str, mode: str = "diagnose",
             directory: str = "~/iac",
             issue: Optional[int] = None) -> dict:
    """Spawn a Claude Code sub-agent detached. Returns job + log info."""
    task = (task or "").strip()
    if not task:
        return {"status": "error", "detail": "empty task"}
    mode = mode if mode in ("diagnose", "apply", "plan") else "diagnose"
    wd = os.path.expanduser(directory or "~/iac")
    cmd = _build_cmd(task, mode, wd, issue)

    os.makedirs(_LOG_DIR, exist_ok=True)
    job_id = f"job-{int(time.time())}-{os.getpid()}"
    log_path = os.path.join(_LOG_DIR, f"{job_id}.log")
    try:
        # Detach from the MCP server's process group so the sub-agent keeps
        # running after the brain's MCP call returns.
        with open(log_path, "wb") as logf:
            proc = subprocess.Popen(
                ["bash", "-lc", cmd],
                stdout=logf, stderr=subprocess.STDOUT,
                stdin=subprocess.DEVNULL,
                start_new_session=True,
            )
    except (OSError, subprocess.SubprocessError) as exc:
        return {"status": "error", "detail": f"spawn failed: {exc}"}

    posture = {"apply": "applying changes",
               "plan": "plan only",
               "diagnose": "read-only investigation"}[mode]
    note = (f" on `{_JARVIS_BRANCH}` branch"
            f"{' (tracking issue #' + str(issue) + ')' if issue else ''}"
            if mode == "apply" else "")
    return {
        "status": "ok",
        "job_id": job_id,
        "pid": proc.pid,
        "log_path": log_path,
        "mode": mode,
        "posture": posture + note,
        "issue": issue,
        "directory": wd,
        "message": (f"Sub-agent spawned (job {job_id}, {posture}{note}, "
                    f"$2 cap). Track via the issue comments or `tail -f "
                    f"{log_path}`."),
    }


# ── MCP server boilerplate (same shape as jarvis_personal_mcp.py) ────────
_TOOLS = [
    {
        "name": "delegate",
        "description": (
            "Spawn a Claude Code sub-agent to do work autonomously. Use "
            "for multi-step coding, deployments, debugging, anything you "
            "can't do directly with your read-only tools.\n\n"
            "Modes:\n"
            "  • diagnose (default) — read-only investigation; lets the "
            "agent run kubectl get/describe/logs, git log/diff, curl, "
            "etc. Use for 'why is X broken'.\n"
            "  • apply — agent may change files, run kubectl apply, "
            "commit and push. **Only use when the user explicitly said "
            "make / apply / fix / deploy / commit.** "
            "Auto-checks-out the `jarvis` branch and posts a starting "
            "comment on the issue if `issue` is given. The agent is "
            "forbidden by its system prompt from pushing to main or "
            "closing the issue.\n"
            "  • plan — propose only, no commands at all.\n\n"
            "If you're applying work, pass the GitHub `issue` number you "
            "(or the user) opened so the agent comments on it. If no "
            "issue exists yet, open one first with `gh issue create` "
            "then delegate with that issue number.\n\n"
            "Spawns detached and returns immediately. Track progress via "
            "the issue comments or the returned `log_path`."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "task": {"type": "string",
                          "description": "Clear, self-contained task description for the sub-agent."},
                "mode": {"type": "string",
                          "enum": ["diagnose", "apply", "plan"],
                          "description": "diagnose | apply | plan (default diagnose)"},
                "directory": {"type": "string",
                              "description": "Working directory; defaults to ~/iac."},
                "issue": {"type": "integer",
                           "description": "Optional GitHub issue number in the repo at `directory`. When provided + mode='apply', the daemon checks out `jarvis`, posts a starting comment, and the sub-agent will comment a summary on completion."},
            },
            "required": ["task"],
            "additionalProperties": False,
        },
    },
]


def _text_result(text: str) -> dict:
    return {"content": [{"type": "text", "text": text}]}


def _call(name: str, args: dict) -> dict:
    if name == "delegate":
        return _text_result(json.dumps(delegate(
            task=args.get("task", ""),
            mode=args.get("mode", "diagnose"),
            directory=args.get("directory", "~/iac"),
            issue=args.get("issue"),
        )))
    return {"content": [{"type": "text", "text": f"unknown tool: {name}"}],
            "isError": True}


def _handle(req: dict):
    method = req.get("method")
    rid = req.get("id")
    if method == "initialize":
        return {"jsonrpc": "2.0", "id": rid, "result": {
            "protocolVersion": "2025-11-25",
            "capabilities": {"tools": {"listChanged": False}},
            "serverInfo": {"name": "jarvis_delegate", "version": "0.1.0"},
        }}
    if method == "notifications/initialized":
        return None
    if method == "tools/list":
        return {"jsonrpc": "2.0", "id": rid, "result": {"tools": _TOOLS}}
    if method == "tools/call":
        params = req.get("params") or {}
        try:
            return {"jsonrpc": "2.0", "id": rid,
                    "result": _call(params.get("name", ""),
                                     params.get("arguments") or {})}
        except Exception as exc:  # noqa: BLE001
            return {"jsonrpc": "2.0", "id": rid,
                    "error": {"code": -32603,
                              "message": f"{type(exc).__name__}: {exc}",
                              "data": traceback.format_exc()}}
    if rid is None:
        return None
    return {"jsonrpc": "2.0", "id": rid,
            "error": {"code": -32601, "message": f"method not found: {method}"}}


def main() -> None:
    out = sys.stdout
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
        except json.JSONDecodeError:
            continue
        resp = _handle(req)
        if resp is not None:
            out.write(json.dumps(resp) + "\n")
            out.flush()


if __name__ == "__main__":
    main()
