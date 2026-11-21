import http from "node:http";

const PORT = Number(process.env.YINGCE_AGENT_PORT || 8090);
const TOKEN = process.env.YINGCE_AGENT_TOKEN || "";
const MAX_SESSIONS = Number(process.env.YINGCE_AGENT_MAX_SESSIONS || 8);

let activeSessions = 0;

const server = http.createServer(async (req, res) => {
  res.setHeader("Content-Type", "application/json; charset=utf-8");

  if (req.method === "GET" && req.url === "/health") {
    res.writeHead(200);
    res.end(
      JSON.stringify({
        status: "ok",
        runtime: "yingce-agent-sidecar",
        activeSessions,
        maxSessions: MAX_SESSIONS,
      }),
    );
    return;
  }

  if (TOKEN && req.headers.authorization !== `Bearer ${TOKEN}`) {
    res.writeHead(401);
    res.end(JSON.stringify({ ok: false, error: "unauthorized sidecar token" }));
    return;
  }

  if (req.method === "POST" && req.url === "/v1/sessions/run") {
    if (activeSessions >= MAX_SESSIONS) {
      res.writeHead(429);
      res.end(JSON.stringify({ ok: false, error: "agent session capacity reached" }));
      return;
    }
    activeSessions++;
    try {
      const chunks = [];
      for await (const chunk of req) chunks.push(chunk);
      const body = JSON.parse(Buffer.concat(chunks).toString("utf-8") || "{}");
      res.writeHead(200);
      res.end(
        JSON.stringify({
          ok: true,
          runId: body.runId || "run_local",
          plan: [
            { tool: "canvas.read_snapshot" },
            { tool: "canvas.create_storyboard_workflow", args: { prompt: body.prompt || "" } },
          ],
        }),
      );
    } finally {
      activeSessions--;
    }
    return;
  }

  res.writeHead(404);
  res.end(JSON.stringify({ ok: false, error: "not found" }));
});

server.listen(PORT, () => {
  console.log(`yingce-agent sidecar listening on :${PORT} (maxSessions=${MAX_SESSIONS})`);
});
