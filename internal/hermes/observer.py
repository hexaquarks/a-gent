# Managed by a-gent: native Hermes lifecycle observer.
import atexit
import builtins
import copy
import json
import os
from pathlib import Path
import sys
import threading
import time


def register(ctx):
    # Reloads dispose the previous writer before registering a new one. Keep the
    # lease outside this module because Hermes imports plugins under new names.
    previous = getattr(builtins, "_a_gent_hermes_observer", None)
    inherited = previous.snapshot() if previous is not None else None
    if previous is not None:
        previous()
    import psutil
    from hermes_constants import get_hermes_home

    cache = (Path.home() / "Library" / "Caches" if sys.platform == "darwin" else
             Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache")))
    directory = cache / "a-gent" / "hermes"
    pid = os.getpid()
    started = round(psutil.Process(pid).create_time() * 1000)
    path = directory / "instances" / f"{pid}.json"
    preview_path = directory / "previews" / f"{pid}.json"
    home = get_hermes_home()
    lock = threading.RLock()
    stop = threading.Event()
    record = {"version": 1, "pid": pid, "startedAt": started, "updatedAt": 0,
              "lastActiveAt": 0, "directory": os.getcwd(), "sessionID": "",
              "name": "Hermes", "state": "idle"}
    preview = {"sessionID": "", "directory": record["directory"], "activity": None, "edit": None}
    lease_session = ""
    stream_key = None
    stream_text = ""
    completed_turns = []
    if inherited is not None:
        old_record, old_preview, lease_session = inherited
        record.update(old_record)
        preview.update(old_preview)

    def write(target, value):
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        temporary = target.with_suffix(".tmp")
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as file:
            json.dump(value, file)
        os.replace(temporary, target)

    def publish():
        if record["sessionID"]:
            record["updatedAt"] = round(time.time() * 1000)
            write(preview_path, preview)
            write(path, record)

    def select(session):
        nonlocal stream_key, stream_text
        if session and session != record["sessionID"]:
            record.update(sessionID=session, state="idle", lastActiveAt=round(time.time() * 1000))
            preview.update(sessionID=session, activity=None, edit=None)
            stream_key, stream_text = None, ""
            completed_turns.clear()

    def refresh_lease():
        nonlocal lease_session
        # The native registry also records an idle CLI before its first turn.
        # Rebind only when the lease changes; compression can change the agent's
        # session ID before the CLI updates its lease at the end of that turn.
        entries = json.loads((home / "runtime" / "active_sessions.json").read_text()).get("entries", [])
        for entry in entries:
            if entry.get("pid") == pid and entry.get("surface") == "cli":
                session = entry.get("session_id", "")
                if session != lease_session:
                    lease_session = session
                    select(session)
                break

    def heartbeat():
        while not stop.is_set():
            try:
                with lock:
                    refresh_lease()
                    publish()
            except Exception:
                # Storage/registry failure never interrupts Hermes; the dashboard
                # rejects stale metadata instead of guessing a status.
                pass
            stop.wait(1)

    def event(event_type, **data):
        nonlocal stream_key, stream_text
        if stop.is_set():
            return
        try:
            with lock:
                session = data.get("session_id") or data.get("session_key")
                if event_type == "reset":
                    select(session)
                    preview.update(activity=None, edit=None)
                elif event_type == "command":
                    if data.get("command", "").lstrip("/") not in ("undo", "rollback", "new", "reset", "resume"):
                        return
                    preview.update(activity=None, edit=None)
                elif event_type == "start":
                    if data.get("platform") not in (None, "", "cli"):
                        return
                    select(session)
                elif session and session != record["sessionID"]:
                    return  # Another agent's tools and output cannot enter this preview.
                if not record["sessionID"]:
                    return
                now = round(time.time() * 1000)
                record["lastActiveAt"] = now
                if event_type in ("start", "running"):
                    record["state"] = "running"
                elif event_type == "waiting":
                    record["state"] = "waiting"
                elif event_type == "end":
                    record["state"] = "idle"
                    turn = data.get("turn_id")
                    if turn:
                        completed_turns.append(turn)
                        del completed_turns[:-8]
                elif event_type == "delta":
                    turn = data.get("turn_id")
                    if data.get("kind") != "text" or not turn or turn in completed_turns:
                        return
                    delta = data.get("delta")
                    if not isinstance(delta, str) or not delta:
                        return
                    key = (turn, data.get("iteration"))
                    if key != stream_key:
                        stream_key, stream_text = key, ""
                    stream_text = (stream_text + delta)[-8192:]
                    preview["activity"] = {"label": "Assistant", "text": stream_text, "at": now}
                elif event_type == "interim":
                    text = data.get("text")
                    if isinstance(text, str) and text.strip():
                        preview["activity"] = {"label": "Assistant", "text": text[-8192:], "at": now}
                elif event_type == "assistant":
                    turn = data.get("turn_id")
                    if turn:
                        completed_turns.append(turn)
                        del completed_turns[:-8]
                    text = data.get("assistant_response")
                    if isinstance(text, str) and text.strip():
                        preview["activity"] = {"label": "Assistant", "text": text[-8192:], "at": now}
                elif event_type == "tool":
                    result = data.get("result")
                    if isinstance(result, str):
                        try:
                            result = json.loads(result)
                        except ValueError:
                            result = {"output": result}
                    if isinstance(result, dict):
                        tool = data.get("tool_name", "Tool")
                        text = result.get("output") or result.get("message") or result.get("diff")
                        if isinstance(text, str) and text.strip():
                            preview["activity"] = {"label": tool, "text": text[-8192:], "at": now}
                        # Only a successful native patch result proves an edit.
                        diff = result.get("diff")
                        files = result.get("files_modified")
                        if (tool == "patch" and data.get("status") == "ok" and
                                result.get("success") is True and isinstance(diff, str) and diff and
                                isinstance(files, list) and len(files) == 1 and isinstance(files[0], str)):
                            preview["edit"] = {"filename": files[0], "diff": diff[:32768],
                                               "at": now, "truncated": len(diff) > 32768}
                publish()
        except Exception:
            pass

    thread = threading.Thread(target=heartbeat, name="a-gent-hermes", daemon=True)

    def close():
        stop.set()
        if thread.is_alive() and threading.current_thread() is not thread:
            thread.join(timeout=2)
        with lock:
            for target in (path, preview_path):
                try:
                    target.unlink(missing_ok=True)
                except OSError:
                    pass

    def snapshot():
        with lock:
            return dict(record), copy.deepcopy(preview), lease_session

    close.snapshot = snapshot
    builtins._a_gent_hermes_observer = close
    if hasattr(ctx, "on_unload"):
        ctx.on_unload(close)
    atexit.register(close)
    for hook, kind in (("on_session_start", "start"), ("on_session_reset", "reset"),
                       ("pre_command", "command"), ("pre_llm_call", "running"),
                       ("on_session_end", "end"), ("post_llm_call", "assistant"),
                       ("post_tool_call", "tool"), ("on_stream_delta", "delta"),
                       ("on_interim_message", "interim"), ("pre_approval_request", "waiting"),
                       ("post_approval_response", "running"), ("on_human_input_request", "waiting"),
                       ("on_human_input_resolved", "running")):
        ctx.register_hook(hook, lambda _kind=kind, **data: event(_kind, **data))
    thread.start()
