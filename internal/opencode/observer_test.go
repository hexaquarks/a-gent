package opencode

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureObserverPreservesSettingsAndOtherPlugins(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OPENCODE_CONFIG_DIR", root)
	settings := filepath.Join(root, "opencode.jsonc")
	original := []byte("{\n // preserve comments\n \"model\": \"custom/model\"\n}\n")
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureObserver(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settings)
	if err != nil || string(data) != string(original) {
		t.Fatalf("settings changed: %v", err)
	}
	path := filepath.Join(root, "plugins", "a-gent.ts")
	if err := os.WriteFile(path, []byte("export default function customPlugin() {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureObserver(); err == nil {
		t.Fatal("overwrote unrelated plugin")
	}
}

func TestObserverNativeReadsAndLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the plugin harness")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "observer.mjs"), observer, 0600); err != nil {
		t.Fatal(err)
	}
	harness := `
import assert from "node:assert/strict";
import observer from "./observer.mjs";

let handler;
let starts = 0;
let stops = 0;

globalThis.Bun = {
    serve(options) {
        assert.equal(options.hostname, "127.0.0.1");
        assert.equal(options.port, 0);
        handler = options.fetch;
        starts++;

        return {
            stop(force) {
                assert.equal(force, true);
                stops++;
            },
        };
    },
};

const client = {
    session: {
        async list() {
            return { data: [{ id: "own-session" }] };
        },
        async status() {
            return { data: { "own-session": { type: "busy" } } };
        },
        async messages(options) {
            assert.equal(options.path.id, "own-session");
            return { data: ["public-output"] };
        },
    },
};

const hooks = await observer({ client, directory: "/project" });
const second = await observer({ client, directory: "/second" });
assert.equal(starts, 1);

const request = (path, method = "GET", directory = "/project") => {
    const url = "http://127.0.0.1" + path + "?directory=" + encodeURIComponent(directory);
    return handler(new Request(url, { method }));
};

assert.deepEqual(await (await request("/session")).json(), [{ id: "own-session" }]);
assert.deepEqual(await (await request("/session/status")).json(), { "own-session": { type: "busy" } });
assert.deepEqual(await (await request("/session/own-session/message")).json(), ["public-output"]);
assert.equal((await request("/session", "POST")).status, 405);
assert.equal((await request("/unknown")).status, 404);
assert.equal((await request("/session", "GET", "/unobserved")).status, 404);

await hooks.event({
    event: {
        type: "permission.asked",
        properties: { id: "p", sessionID: "own-session" },
    },
});
await hooks.event({
    event: {
        type: "question.asked",
        properties: { id: "q", sessionID: "own-session" },
    },
});
assert.equal((await (await request("/permission")).json()).length, 1);
assert.equal((await (await request("/question")).json()).length, 1);
assert.equal((await (await request("/permission", "GET", "/second")).json()).length, 0);

await hooks.event({
    event: { type: "permission.replied", properties: { requestID: "p" } },
});
await hooks.event({
    event: { type: "question.rejected", properties: { requestID: "q" } },
});
assert.deepEqual(await (await request("/permission")).json(), []);
assert.deepEqual(await (await request("/question")).json(), []);

client.session.status = async () => {
    throw new Error("unavailable");
};
assert.equal((await request("/session/status")).status, 503);

await hooks.dispose();
assert.equal(stops, 0);
await second.dispose();
assert.equal(stops, 1);

console.log("native bridge lifecycle passed");
`
	path := filepath.Join(root, "harness.mjs")
	if err := os.WriteFile(path, []byte(harness), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, path).CombinedOutput()
	if err != nil || !strings.Contains(string(output), "native bridge lifecycle passed") {
		t.Fatalf("harness: %v\n%s", err, output)
	}
}

func TestConfigureObserverUsesConfigHomeAndRefreshesManagedPlugin(t *testing.T) {
	for _, useConfigHome := range []bool{false, true} {
		name := "default home"
		if useConfigHome {
			name = "XDG config home"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", root)
			t.Setenv("OPENCODE_CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			configuration := filepath.Join(root, ".config")
			if useConfigHome {
				configuration = filepath.Join(root, "custom config")
				t.Setenv("XDG_CONFIG_HOME", configuration)
			}
			directory := filepath.Join(configuration, "opencode", "plugins")
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(directory, "a-gent.ts")
			if err := os.WriteFile(path, []byte("// OpenCode's plain TUI: previous observer\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			if err := ConfigureObserver(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != string(observer) {
				t.Fatalf("managed plugin not refreshed: %v", err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := ConfigureObserver(); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatalf("identical plugin replaced: %v", err)
			}
		})
	}
}
