// OpenCode's plain TUI has an in-process API, but no HTTP listener. Expose only
// the native reads needed by the existing adapter, through the injected SDK.
export default async function ({ client, directory }) {
    const key = Symbol.for("a-gent.opencode.observer");

    // OpenCode can load separate project contexts in the same worker. Share one
    // listener, but route each read to that project's own SDK and pending events.
    const runtime = globalThis[key] ??= { contexts: new Map(), server: null };
    const context = { client, permissions: new Map(), questions: new Map() };
    runtime.contexts.set(directory, context);

    if (!runtime.server) {
        try {
            runtime.server = Bun.serve({
                hostname: "127.0.0.1",
                port: 0,
                async fetch(request) {
                    const url = new URL(request.url);
                    const current = runtime.contexts.get(url.searchParams.get("directory"));

                    if (request.method !== "GET") {
                        return new Response(null, { status: 405 });
                    }

                    if (!current) {
                        return new Response(null, { status: 404 });
                    }

                    const options = {
                        query: { directory: url.searchParams.get("directory") },
                        throwOnError: true,
                    };

                    try {
                        let data;

                        if (url.pathname === "/session") {
                            data = (await current.client.session.list(options)).data;
                        } else if (url.pathname === "/session/status") {
                            data = (await current.client.session.status(options)).data;
                        } else if (url.pathname === "/permission") {
                            data = [...current.permissions.values()];
                        } else if (url.pathname === "/question") {
                            data = [...current.questions.values()];
                        } else {
                            const match = /^\/session\/([^/]+)\/message$/.exec(url.pathname);

                            if (!match) {
                                return new Response(null, { status: 404 });
                            }

                            data = (await current.client.session.messages({
                                ...options,
                                path: { id: match[1] },
                            })).data;
                        }

                        return Response.json(data);
                    } catch {
                        return new Response(null, { status: 503 });
                    }
                },
            });
        } catch {
            // Observation must never prevent OpenCode from starting.
        }
    }

    return {
        async event({ event }) {
            const properties = event.properties;

            switch (event.type) {
                case "permission.asked":
                case "permission.updated":
                    context.permissions.set(properties.id, properties);
                    break;

                case "permission.replied":
                    context.permissions.delete(properties.requestID ?? properties.permissionID);
                    break;

                case "question.asked":
                    context.questions.set(properties.id, properties);
                    break;

                case "question.replied":
                case "question.rejected":
                    context.questions.delete(properties.requestID);
                    break;
            }
        },

        async dispose() {
            if (runtime.contexts.get(directory) === context) {
                runtime.contexts.delete(directory);
            }

            if (runtime.contexts.size === 0) {
                runtime.server?.stop(true);
                runtime.server = null;
            }
        },
    };
}
