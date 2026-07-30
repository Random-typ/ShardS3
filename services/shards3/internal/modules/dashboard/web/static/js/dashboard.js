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

document.addEventListener("DOMContentLoaded", () => {
  startHealthStream();
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