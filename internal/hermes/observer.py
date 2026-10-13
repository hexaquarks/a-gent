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

    cache = (
        Path.home() / "Library" / "Caches"
        if sys.platform == "darwin"
        else Path(os.environ.get("XDG_CACHE_HOME", Path.home() / ".cache"))
    )
    directory = cache / "a-gent" / "hermes"
    pid = os.getpid()
    started = round(psutil.Process(pid).create_time() * 1000)
    path = directory / "instances" / f"{pid}.json"
    preview_path = directory / "previews" / f"{pid}.json"
    home = get_hermes_home()

    lock = threading.RLock()
    stop = threading.Event()

    record = {
        "version": 1,
        "pid": pid,
        "startedAt": started,
        "updatedAt": 0,
        "lastActiveAt": 0,
        "directory": os.getcwd(),
        "sessionID": "",
        "name": "Hermes",
        "state": "idle",
    }
    preview = {
        "sessionID": "",
        "directory": record["directory"],
        "activity": None,
        "edit": None,
    }

    lease_session = ""
    stream_key = None
    stream_text = ""
    completed_turns = []
    children = {}

    if inherited is not None:
        old_record, old_preview, lease_session, old_children = inherited
        record.update(old_record)
        preview.update(old_preview)
        children.update(old_children)

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
            record["children"] = [child["record"] for child in children.values()]
            for session, child in children.items():
                write(directory / "previews" / f"{pid}-{session}.json", child["preview"])
            write(preview_path, preview)
            write(path, record)

    def clear_children():
        for old_session in children:
            (directory / "previews" / f"{pid}-{old_session}.json").unlink(missing_ok=True)
        children.clear()

    def select(session):
        nonlocal stream_key, stream_text

        if session and session != record["sessionID"]:
            record.update(sessionID=session, state="idle", lastActiveAt=round(time.time() * 1000))
            preview.update(sessionID=session, activity=None, edit=None)
            stream_key, stream_text = None, ""
            completed_turns.clear()
            clear_children()

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

    # Root and delegated sessions use the same native public-event decoder.
    def update(target, event_type, data):
        now = round(time.time() * 1000)
        target["record"]["lastActiveAt"] = now

        if event_type in ("start", "running"):
            target["record"]["state"] = "running"

        elif event_type == "waiting":
            target["record"]["state"] = "waiting"

        elif event_type == "end":
            target["record"]["state"] = "idle"

            turn = data.get("turn_id")
            if turn:
                target["completed_turns"].append(turn)
                del target["completed_turns"][:-8]

        elif event_type == "delta":
            turn = data.get("turn_id")
            if data.get("kind") != "text" or not turn or turn in target["completed_turns"]:
                return

            delta = data.get("delta")
            if not isinstance(delta, str) or not delta:
                return

            key = (turn, data.get("iteration"))
            if key != target["stream_key"]:
                target["stream_key"], target["stream_text"] = key, ""

            target["stream_text"] = (target["stream_text"] + delta)[-8192:]
            target["preview"]["activity"] = {"label": "Assistant", "text": target["stream_text"], "at": now}

        elif event_type == "interim":
            text = data.get("text")
            if isinstance(text, str) and text.strip():
                target["preview"]["activity"] = {"label": "Assistant", "text": text[-8192:], "at": now}

        elif event_type == "assistant":
            turn = data.get("turn_id")
            if turn:
                target["completed_turns"].append(turn)
                del target["completed_turns"][:-8]

            text = data.get("assistant_response")
            if isinstance(text, str) and text.strip():
                target["preview"]["activity"] = {"label": "Assistant", "text": text[-8192:], "at": now}

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
                    target["preview"]["activity"] = {"label": tool, "text": text[-8192:], "at": now}

                # Only a successful native patch result proves an edit.
                diff = result.get("diff")
                files = result.get("files_modified")
                if (
                    tool == "patch"
                    and data.get("status") == "ok"
                    and result.get("success") is True
                    and isinstance(diff, str)
                    and diff
                    and isinstance(files, list)
                    and len(files) == 1
                    and isinstance(files[0], str)
                ):
                    target["preview"]["edit"] = {
                        "filename": files[0],
                        "diff": diff[:32768],
                        "at": now,
                        "truncated": len(diff) > 32768,
                    }

    def event(event_type, **data):
        nonlocal stream_key, stream_text

        if stop.is_set():
            return

        try:
            with lock:
                session = data.get("session_id") or data.get("session_key")

                if event_type == "child_start":
                    parent = data.get("parent_session_id")
                    session = data.get("child_session_id")
                    if (
                        not record["sessionID"]
                        or not isinstance(session, str)
                        or not session
                        or session in (".", "..", record["sessionID"], parent)
                        or "/" in session
                        or "\\" in session
                        or parent not in (record["sessionID"], *children)
                    ):
                        return

                    if session not in children:
                        children[session] = {
                            "record": {
                                "sessionID": session, "parentID": parent,
                                "name": data.get("child_goal", "Hermes subagent")[:200],
                                "directory": record["directory"], "state": "running",
                                "lastActiveAt": round(time.time() * 1000),
                            },
                            "preview": {
                                "sessionID": session, "directory": record["directory"],
                                "activity": None, "edit": None,
                            },
                            "stream_key": None, "stream_text": "", "completed_turns": [],
                        }
                    publish()
                    return

                if event_type == "child_stop":
                    session = data.get("child_session_id")
                    if session not in children:
                        return

                    target = children[session]
                    target["record"]["state"] = (
                        "idle" if data.get("child_status") == "completed" else "error"
                    )
                    target["record"]["lastActiveAt"] = round(time.time() * 1000)
                    summary = data.get("child_summary")
                    if isinstance(summary, str) and summary.strip():
                        target["preview"]["activity"] = {
                            "label": "Assistant", "text": summary[-8192:],
                            "at": target["record"]["lastActiveAt"],
                        }
                    publish()
                    return

                if event_type == "reset":
                    if session in children or data.get("platform") == "subagent":
                        return
                    select(session)
                    clear_children()
                    preview.update(activity=None, edit=None)

                elif event_type == "command":
                    if data.get("command", "").lstrip("/") not in ("undo", "rollback", "new", "reset", "resume"):
                        return

                    preview.update(activity=None, edit=None)

                elif event_type == "start":
                    if data.get("platform") not in (None, "", "cli"):
                        return

                    select(session)

                elif session and session != record["sessionID"] and session not in children:
                    return  # Another agent's tools and output cannot enter this preview.

                if not record["sessionID"]:
                    return

                if session in children:
                    target = children[session]
                else:
                    target = {
                        "record": record, "preview": preview,
                        "stream_key": stream_key, "stream_text": stream_text,
                        "completed_turns": completed_turns,
                    }

                update(target, event_type, data)
                if target["record"] is record:
                    stream_key, stream_text = target["stream_key"], target["stream_text"]

                publish()
        except Exception:
            pass

    thread = threading.Thread(target=heartbeat, name="a-gent-hermes", daemon=True)

    def close():
        with lock:
            if stop.is_set():
                return
            stop.set()
        if thread.is_alive() and threading.current_thread() is not thread:
            thread.join(timeout=2)

        with lock:
            child_paths = [
                directory / "previews" / f"{pid}-{session}.json"
                for session in children
            ]
            for target in (path, preview_path, *child_paths):
                try:
                    target.unlink(missing_ok=True)
                except OSError:
                    pass

    def snapshot():
        with lock:
            return dict(record), copy.deepcopy(preview), lease_session, copy.deepcopy(children)

    close.snapshot = snapshot
    builtins._a_gent_hermes_observer = close
    if hasattr(ctx, "on_unload"):
        ctx.on_unload(close)
    atexit.register(close)

    for hook, kind in (
        ("subagent_start", "child_start"),
        ("subagent_stop", "child_stop"),
        ("on_session_start", "start"),
        ("on_session_reset", "reset"),
        ("pre_command", "command"),
        ("pre_llm_call", "running"),
        ("on_session_end", "end"),
        ("post_llm_call", "assistant"),
        ("post_tool_call", "tool"),
        ("on_stream_delta", "delta"),
        ("on_interim_message", "interim"),
        ("pre_approval_request", "waiting"),
        ("post_approval_response", "running"),
        ("on_human_input_request", "waiting"),
        ("on_human_input_resolved", "running"),
    ):
        ctx.register_hook(hook, lambda _kind=kind, **data: event(_kind, **data))

    thread.start()
