package pi

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestObserverPublishesNativeLifecycleAndCleansUp(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("observer contract test requires Node.js")
	}
	directory := t.TempDir()
	// The extension contains plain JavaScript so this runs on Node versions
	// without a TypeScript loader, using exactly the bytes embedded in Go.
	path := filepath.Join(directory, "observer.mjs")
	if err := os.WriteFile(path, observer, 0600); err != nil {
		t.Fatal(err)
	}
	script := `
import { readFileSync, existsSync } from "node:fs";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
import assert from "node:assert/strict";
const path = process.argv[1];
const handlers = new Map();
const pi = { on(event, handler) { handlers.set(event, handler); } };
let idle = true, pending = false, session = "one", leaf = "first";
const ctx = { cwd: "/project", isIdle: () => idle, hasPendingMessages: () => pending,
  sessionManager: { getSessionId: () => session, getSessionFile: () => "/project/one.jsonl",
    getLeafId: () => leaf, getSessionName: () => "Pi test" } };
const observer = (await import(pathToFileURL(path))).default;
observer(pi);
const registrations = handlers.size;
observer({ on() { throw new Error("observer registered twice"); } });
assert.equal(handlers.size, registrations);
const file = join(process.argv[2], "instances", String(process.pid) + ".json");
assert.equal(existsSync(file), false);
handlers.get("session_start")({}, ctx);
const read = () => JSON.parse(readFileSync(file, "utf8"));
assert.equal(read().state, "idle");
const first = read();
handlers.get("agent_start")({}, ctx);
assert.equal(read().state, "running");
idle = false;
leaf = "second";
handlers.get("message_end")({}, ctx);
assert.equal(read().leafID, "second");
assert.equal(read().state, "running");
const activeAt = read().lastActiveAt;
idle = true;
await new Promise(resolve => setTimeout(resolve, 1100));
assert.equal(read().state, "idle");
assert.equal(read().lastActiveAt, activeAt);
pending = true;
await new Promise(resolve => setTimeout(resolve, 1100));
assert.equal(read().state, "running");
handlers.get("session_shutdown")({}, ctx);
assert.equal(existsSync(file), false);
pending = false; session = "two";
observer(pi);
handlers.get("session_start")({}, ctx);
assert.equal(read().sessionID, "two");
assert.equal(read().startedAt, first.startedAt);
handlers.get("session_shutdown")({}, ctx);
handlers.get("session_shutdown")({}, ctx);
assert.equal(existsSync(file), false);
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, path, directory).CombinedOutput()
	if err != nil {
		t.Fatalf("observer lifecycle: %v\n%s", err, output)
	}
}
