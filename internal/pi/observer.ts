import { mkdirSync, writeFileSync, renameSync, unlinkSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// Only native session metadata leaves Pi. Prompts, reasoning, and tool arguments
// stay in its transcript and are read only when that session is selected.
export default function (pi) {
    // Pi may load both its user extension and a-gent's explicit launch argument.
    // One shared lease prevents duplicate handlers and heartbeat writers.
    const key = Symbol.for("a-gent.pi.observer");

    if (globalThis[key]) {
        return;
    }

    const owner = {};
    globalThis[key] = owner;

    const directory = join(dirname(fileURLToPath(import.meta.url)), "instances");
    const path = join(directory, `${process.pid}.json`);
    const birthKey = Symbol.for("a-gent.pi.startedAt");
    const startedAt = globalThis[birthKey] ??
        (globalThis[birthKey] = Math.round(Date.now() - process.uptime() * 1000));

    let timer;
    let lastActiveAt = Date.now();
    let context;

    function publish(ctx, state) {
        context = ctx;

        try {
            const record = {
                version: 1,
                pid: process.pid,
                startedAt,
                updatedAt: Date.now(),
                lastActiveAt,
                directory: ctx.cwd,
                sessionID: ctx.sessionManager.getSessionId(),
                sessionFile: ctx.sessionManager.getSessionFile() ?? "",
                leafID: ctx.sessionManager.getLeafId(),
                name: ctx.sessionManager.getSessionName() ?? "Pi",
                state: state ?? (ctx.isIdle() && !ctx.hasPendingMessages() ? "idle" : "running"),
            };

            mkdirSync(directory, { recursive: true, mode: 0o700 });
            writeFileSync(`${path}.tmp`, JSON.stringify(record), { mode: 0o600 });
            renameSync(`${path}.tmp`, path);
        } catch {
            // Storage failures must not interrupt Pi. a-gent detects a stale heartbeat.
        }
    }

    pi.on("session_start", (_event, ctx) => {
        globalThis[key] = owner;
        lastActiveAt = Date.now();
        publish(ctx);

        clearInterval(timer);
        timer = setInterval(() => {
            publish(context);
        }, 1000);

        timer.unref();
    });

    for (const event of ["session_tree", "session_compact"]) {
        pi.on(event, (_event, ctx) => {
            lastActiveAt = Date.now();
            publish(ctx);
        });
    }

    pi.on("agent_start", (_event, ctx) => {
        lastActiveAt = Date.now();
        publish(ctx, "running");
    });

    for (const event of ["message_end", "tool_execution_end", "agent_end"]) {
        pi.on(event, (_event, ctx) => {
            lastActiveAt = Date.now();
            publish(ctx);
        });
    }

    pi.on("session_shutdown", () => {
        clearInterval(timer);

        if (globalThis[key] === owner) {
            delete globalThis[key];
        }

        for (const file of [path, `${path}.tmp`]) {
            try {
                unlinkSync(file);
            } catch {
                // A failed publish may leave no metadata file to remove.
            }
        }
    });
}
