const elements = {
  refreshButton: document.getElementById("refresh-all"),
  syncErrorBox: document.getElementById("sync-error-box"),
  routeCount: document.getElementById("route-count"),
  tlsCount: document.getElementById("tls-count"),
  configPath: document.getElementById("config-path"),
  serviceState: document.getElementById("service-state"),
  lastSync: document.getElementById("last-sync"),
  lastValidation: document.getElementById("last-validation"),
  lastReload: document.getElementById("last-reload"),
  lastCertificate: document.getElementById("last-certificate"),
  routeTableMeta: document.getElementById("route-table-meta"),
  routesBody: document.getElementById("routes-body"),
};

boot();

function boot() {
  elements.refreshButton.addEventListener("click", refreshAll);
  refreshAll();
}

async function refreshAll() {
  try {
    const [statusPayload, routesPayload] = await Promise.all([
      requestJSON("/status"),
      requestJSON("/routes"),
    ]);

    renderStatus(statusPayload);
    renderRoutes(routesPayload.routes || []);
  } catch (error) {
    renderGlobalError(error.message);
  }
}

function renderStatus(payload) {
  elements.routeCount.textContent = String(payload.route_count ?? 0);
  elements.tlsCount.textContent = String(payload.tls_ready_count ?? 0);
  elements.configPath.textContent = payload.sync?.config_path || "-";

  const status = payload.status || "unknown";
  elements.serviceState.textContent = status;
  elements.serviceState.className = `status-pill ${status === "ok" ? "ok" : status === "degraded" ? "degraded" : "neutral"}`;

  elements.lastSync.textContent = formatSyncLine(payload.sync?.last_sync_success_at, payload.sync?.last_sync_error);
  elements.lastValidation.textContent = formatSyncLine(payload.sync?.last_validation_at, payload.sync?.last_validation_error);
  elements.lastReload.textContent = formatSyncLine(payload.sync?.last_reload_at, payload.sync?.last_reload_error);
  elements.lastCertificate.textContent = formatCertificateLine(
    payload.sync?.last_certificate_domain,
    payload.sync?.last_certificate_at,
    payload.sync?.last_certificate_error,
  );

  const syncErrors = [
    payload.sync?.last_sync_error,
    payload.sync?.last_validation_error,
    payload.sync?.last_reload_error,
    payload.sync?.last_certificate_error,
  ].filter(Boolean);

  if (syncErrors.length > 0) {
    elements.syncErrorBox.classList.remove("hidden");
    elements.syncErrorBox.textContent = syncErrors.join(" | ");
  } else {
    elements.syncErrorBox.classList.add("hidden");
    elements.syncErrorBox.textContent = "";
  }
}

function renderRoutes(routes) {
  elements.routeTableMeta.textContent = `${routes.length} loaded`;

  if (routes.length === 0) {
    elements.routesBody.innerHTML = `<tr><td colspan="5" class="empty-state">No routes configured yet. Use the SSH menu to create the first one.</td></tr>`;
    return;
  }

  elements.routesBody.innerHTML = "";
  for (const route of routes) {
    const row = document.createElement("tr");
    row.innerHTML = `
      <td>
        <div class="route-domain">
          <strong>${escapeHTML(route.name)}</strong>
          <small>${escapeHTML(frontendSummary(route))}</small>
        </div>
      </td>
      <td>${escapeHTML(frontendSummary(route))}</td>
      <td>
        <div>${escapeHTML(upstreamSummary(route))}</div>
        <small class="target-meta">${escapeHTML(extraUpstreamSummary(route))}</small>
      </td>
      <td><span class="tls-pill ${route.tls_ready ? "ready" : "pending"}">${route.tls_ready ? "Ready" : route.enable_tls ? "Pending" : "Off"}</span></td>
      <td>${escapeHTML(formatTimestamp(route.updated_at))}</td>
    `;
    elements.routesBody.appendChild(row);
  }
}

function frontendSummary(route) {
  if (route.frontend_mode === "port") {
    if (route.listen_ip) {
      return `${route.listen_ip}:${route.listen_port}`;
    }
    return `0.0.0.0:${route.listen_port}`;
  }

  return route.domain || "-";
}

function upstreamSummary(route) {
  if (route.upstream_mode === "host") {
    return `${route.target_scheme || "http"}://${route.target_host}${suffixPort(route.target_scheme, route.target_port)}`;
  }

  return `${route.target_ip}:${route.target_port}`;
}

function extraUpstreamSummary(route) {
  const extras = [];
  if (route.upstream_host_header) {
    extras.push(`Host=${route.upstream_host_header}`);
  }
  if (route.upstream_sni) {
    extras.push(`SNI=${route.upstream_sni}`);
  }
  if (route.cert_path) {
    extras.push(route.cert_path);
  }
  return extras.join(" | ") || "No extra upstream metadata";
}

function suffixPort(scheme, port) {
  if (!port) {
    return "";
  }
  if ((scheme === "http" && port === 80) || (scheme === "https" && port === 443)) {
    return "";
  }
  return `:${port}`;
}

async function requestJSON(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.error || `Request failed with status ${response.status}`);
  }

  return payload;
}

function renderGlobalError(message) {
  elements.syncErrorBox.classList.remove("hidden");
  elements.syncErrorBox.textContent = message;
}

function formatTimestamp(value) {
  if (!value) {
    return "-";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return date.toLocaleString();
}

function formatSyncLine(timestamp, error) {
  if (error) {
    return `Error: ${error}`;
  }

  if (!timestamp) {
    return "Not run yet";
  }

  return formatTimestamp(timestamp);
}

function formatCertificateLine(domain, timestamp, error) {
  if (error) {
    return `Error: ${error}`;
  }

  if (!domain && !timestamp) {
    return "Not run yet";
  }

  const parts = [];
  if (domain) {
    parts.push(domain);
  }
  if (timestamp) {
    parts.push(formatTimestamp(timestamp));
  }
  return parts.join(" @ ");
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
