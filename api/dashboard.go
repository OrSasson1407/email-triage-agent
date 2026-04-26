package api

// dashboardHTML is the built-in stats dashboard
// Fetches /stats and /logs using the API secret from the URL param:
//   https://your-app.railway.app/dashboard?secret=YOUR_SECRET
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Email Triage Agent</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
           background: #0f172a; color: #e2e8f0; min-height: 100vh; padding: 24px; }
    h1 { font-size: 24px; font-weight: 700; margin-bottom: 4px; }
    .subtitle { color: #64748b; font-size: 14px; margin-bottom: 32px; }
    .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
             gap: 16px; margin-bottom: 32px; }
    .card { background: #1e293b; border-radius: 12px; padding: 20px; }
    .card .label { font-size: 12px; color: #64748b; text-transform: uppercase;
                   letter-spacing: 0.05em; margin-bottom: 8px; }
    .card .value { font-size: 32px; font-weight: 700; }
    .red    { color: #f87171; }
    .yellow { color: #fbbf24; }
    .green  { color: #34d399; }
    .blue   { color: #60a5fa; }
    table { width: 100%; border-collapse: collapse; background: #1e293b;
            border-radius: 12px; overflow: hidden; }
    th { background: #0f172a; padding: 12px 16px; text-align: left;
         font-size: 12px; text-transform: uppercase; color: #64748b; }
    td { padding: 12px 16px; border-top: 1px solid #334155; font-size: 14px; }
    tr:hover td { background: #263348; }
    .badge { display: inline-block; padding: 2px 8px; border-radius: 9999px;
             font-size: 11px; font-weight: 600; }
    .badge-HIGH   { background: #7f1d1d; color: #fca5a5; }
    .badge-MEDIUM { background: #78350f; color: #fde68a; }
    .badge-LOW    { background: #14532d; color: #86efac; }
    .badge-vip    { background: #3b0764; color: #d8b4fe; }
    .refresh { margin-bottom: 24px; }
    button { background: #3b82f6; color: white; border: none; padding: 8px 16px;
             border-radius: 8px; cursor: pointer; font-size: 14px; }
    button:hover { background: #2563eb; }
    .secret-input { background: #1e293b; border: 1px solid #334155; color: #e2e8f0;
                    padding: 8px 12px; border-radius: 8px; font-size: 14px;
                    margin-right: 8px; width: 260px; }
    .error { color: #f87171; margin: 16px 0; }
  </style>
</head>
<body>
  <h1>📧 Email Triage Agent</h1>
  <p class="subtitle" id="last-updated">Loading...</p>

  <div class="refresh">
    <input class="secret-input" type="password" id="secret" placeholder="API Secret" />
    <button onclick="load()">Load Dashboard</button>
  </div>

  <div id="error" class="error" style="display:none;"></div>

  <div class="cards" id="cards">
    <div class="card"><div class="label">Total (24h)</div><div class="value blue" id="total">—</div></div>
    <div class="card"><div class="label">High Urgency</div><div class="value red" id="high">—</div></div>
    <div class="card"><div class="label">Medium</div><div class="value yellow" id="medium">—</div></div>
    <div class="card"><div class="label">Low</div><div class="value green" id="low">—</div></div>
    <div class="card"><div class="label">Processed</div><div class="value blue" id="processed">—</div></div>
    <div class="card"><div class="label">Failed</div><div class="value red" id="failed">—</div></div>
  </div>

  <table id="logs-table">
    <thead>
      <tr>
        <th>From</th>
        <th>Subject</th>
        <th>Urgency</th>
        <th>Topic</th>
        <th>Confidence</th>
        <th>Time</th>
      </tr>
    </thead>
    <tbody id="logs-body">
      <tr><td colspan="6" style="text-align:center; color:#64748b;">Enter your API secret above and click Load</td></tr>
    </tbody>
  </table>

  <script>
    function getSecret() {
      return document.getElementById("secret").value ||
             new URLSearchParams(window.location.search).get("secret") || "";
    }

    async function load() {
      const secret = getSecret();
      const errEl = document.getElementById("error");
      errEl.style.display = "none";

      try {
        const [statsRes, logsRes] = await Promise.all([
          fetch("/stats", { headers: { "X-API-Secret": secret } }),
          fetch("/logs",  { headers: { "X-API-Secret": secret } }),
        ]);

        if (statsRes.status === 401) {
          errEl.textContent = "Invalid API secret.";
          errEl.style.display = "block";
          return;
        }

        const stats = await statsRes.json();
        const logs  = await logsRes.json();

        document.getElementById("total").textContent     = stats.last_24h.total;
        document.getElementById("high").textContent      = stats.last_24h.high;
        document.getElementById("medium").textContent    = stats.last_24h.medium;
        document.getElementById("low").textContent       = stats.last_24h.low;
        document.getElementById("processed").textContent = stats.runtime.processed;
        document.getElementById("failed").textContent    = stats.runtime.failed;
        document.getElementById("last-updated").textContent =
          "Last updated: " + new Date().toLocaleTimeString();

        const tbody = document.getElementById("logs-body");
        tbody.innerHTML = "";

        if (!logs || logs.length === 0) {
          tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#64748b;">No logs yet</td></tr>';
          return;
        }

        logs.forEach(log => {
          const vip = log.IsVIP ? ' <span class="badge badge-vip">VIP</span>' : "";
          const row = document.createElement("tr");
          row.innerHTML =
            "<td>" + (log.From || "—") + "</td>" +
            "<td>" + (log.Subject || "—") + "</td>" +
            '<td><span class="badge badge-' + log.Urgency + '">' + log.Urgency + "</span>" + vip + "</td>" +
            "<td><code>" + (log.Topic || "—") + "</code></td>" +
            "<td>" + Math.round((log.Confidence || 0) * 100) + "%</td>" +
            "<td>" + new Date(log.ProcessedAt).toLocaleString() + "</td>";
          tbody.appendChild(row);
        });

      } catch (e) {
        errEl.textContent = "Error: " + e.message;
        errEl.style.display = "block";
      }
    }

    // Auto-load if secret is in URL param
    if (new URLSearchParams(window.location.search).get("secret")) {
      load();
    }

    // Auto-refresh every 60 seconds
    setInterval(() => {
      if (getSecret()) load();
    }, 60000);
  </script>
</body>
</html>`