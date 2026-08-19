function updateHealthNodes(payload) {
  const isHealthy = Boolean(payload.Ok);
  const message = isHealthy ? (payload.Message || "healthy") : "unhealthy";
  const detail = isHealthy ? `Checked at ${payload.CheckedAt}` : (payload.Error || "Health check failed");

  document.querySelectorAll("[data-health-message]").forEach((node) => {
    node.textContent = message;
  });

  document.querySelectorAll("[data-health-detail]").forEach((node) => {
    node.textContent = detail;
  });

  document.querySelectorAll("[data-health-dot]").forEach((node) => {
    node.classList.remove("bad", "warn");
    if (!isHealthy) {
      node.classList.add("bad");
    }
  });
}

function startHealthStream() {
  const source = new EventSource("/health/longlived");

  source.addEventListener("health", (event) => {
    try {
      updateHealthNodes(JSON.parse(event.data));
    } catch {
      updateHealthNodes({ Ok: false, Error: "Invalid health payload", CheckedAt: "" });
    }
  });

  source.onerror = () => {
    updateHealthNodes({ Ok: false, Error: "Health stream disconnected", CheckedAt: "" });
  };
}

function piePalette(index) {
  const colors = [
    "#0f766e",
    "#1d4ed8",
    "#b45309",
    "#be123c",
    "#7c3aed",
    "#0369a1",
    "#6d28d9",
    "#15803d",
  ];
  return colors[index % colors.length];
}

function renderBackendPieCharts() {
  document.querySelectorAll("[data-backend-pie]").forEach((chartRoot) => {
    const visual = chartRoot.querySelector("[data-backend-pie-visual]");
    const items = Array.from(chartRoot.querySelectorAll("[data-backend-pie-item]"));
    if (!visual || items.length === 0) {
      return;
    }

    const entries = items.map((item) => {
      const bytes = Number(item.getAttribute("data-bytes") || "0");
      const shards = Number(item.getAttribute("data-shards") || "0");
      return { item, bytes, shards };
    });

    const totalBytes = entries.reduce((sum, entry) => sum + Math.max(0, entry.bytes), 0);
    const totalShards = entries.reduce((sum, entry) => sum + Math.max(0, entry.shards), 0);
    const useBytes = totalBytes > 0;
    const total = useBytes ? totalBytes : totalShards;

    if (total <= 0) {
      visual.style.background = "conic-gradient(#e5e7eb 0 100%)";
      items.forEach((item) => {
        const percentNode = item.querySelector(".backend-pie-percent");
        if (percentNode) {
          percentNode.textContent = "0%";
        }
      });
      return;
    }

    let current = 0;
    const segments = [];

    entries.forEach((entry, index) => {
      const value = useBytes ? Math.max(0, entry.bytes) : Math.max(0, entry.shards);
      const percent = (value / total) * 100;
      const start = current;
      const end = current + percent;
      const color = piePalette(index);

      segments.push(`${color} ${start.toFixed(2)}% ${end.toFixed(2)}%`);
      current = end;

      const swatch = entry.item.querySelector(".backend-pie-swatch");
      if (swatch) {
        swatch.style.background = color;
      }

      const percentNode = entry.item.querySelector(".backend-pie-percent");
      if (percentNode) {
        percentNode.textContent = `${percent.toFixed(1)}%`;
      }
    });

    visual.style.background = `conic-gradient(${segments.join(", ")})`;
  });
}

document.addEventListener("DOMContentLoaded", () => {
  startHealthStream();
  renderBackendPieCharts();
});

document.addEventListener("htmx:afterSwap", (event) => {
  if (!event.target || event.target.id !== "buckets-panel") {
    return;
  }

  const input = event.target.querySelector('input[name="bucket_name"]');
  if (input) {
    input.focus();
  }
});