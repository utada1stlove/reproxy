const state = {
  editingDomain: null,
  routes: [],
};

const elements = {
  refreshButton: document.getElementById("refresh-all"),
  resetButton: document.getElementById("reset-form"),
  cancelEditButton: document.getElementById("cancel-edit"),
  form: document.getElementById("route-form"),
  formTitle: document.getElementById("form-title"),
  submitButton: document.getElementById("submit-button"),
  notice: document.getElementById("form-notice"),
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
  domain: document.getElementById("domain"),
  targetIP: document.getElementById("target-ip"),
  targetPort: document.getElementById("target-port"),
};

boot();

function boot() {
  elements.refreshButton.addEventListener("click", () => refreshAll(true));
  elements.resetButton.addEventListener("click", resetForm);
  elements.cancelEditButton.addEventListener("click", resetForm);
  elements.form.addEventListener("submit", onSubmit);

  resetForm();
  refreshAll(false);
}

async function refreshAll(showSuccess) {
  try {
    const [statusPayload, routesPayload] = await Promise.all([
      requestJSON("/status"),
      requestJSON("/routes"),
    ]);

    renderStatus(statusPayload);
    renderRoutes(routesPayload.routes || []);

    if (showSuccess) {
      showNotice("Panel data refreshed.", "success");
    }
  } catch (error) {
    showNotice(error.message, "error");
  }
}

async function onSubmit(event) {
  event.preventDefault();

  const payload = {
    domain: elements.domain.value.trim(),
    target_ip: elements.targetIP.value.trim(),
    target_port: Number.parseInt(elements.targetPort.value, 10),
  };

  if (!payload.domain || !payload.target_ip || Number.isNaN(payload.target_port)) {
    showNotice("Domain, target IP, and target port are required.", "warning");
    return;
  }

  try {
    let response;
    if (state.editingDomain) {
      response = await requestJSON(`/routes/${encodeURIComponent(state.editingDomain)}`, {
        method: "PUT",
        body: JSON.stringify({
          target_ip: payload.target_ip,
          target_port: payload.target_port,
        }),
      });
      showNotice(`Updated ${response.route.domain}.`, "success");
    } else {
      response = await requestJSON("/routes", {
        method: "POST",
        body: JSON.stringify(payload),
      });
      showNotice(`Saved ${response.route.domain}.`, "success");
    }

    resetForm();
    await refreshAll(false);
  } catch (error) {
    showNotice(error.message, "error");
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
  state.routes = routes;
  elements.routeTableMeta.textContent = `${routes.length} loaded`;

  if (routes.length === 0) {
    elements.routesBody.innerHTML = `<tr><td colspan="5" class="empty-state">No routes yet. Create the first one from the form.</td></tr>`;
    return;
  }

  elements.routesBody.innerHTML = "";
  for (const route of routes) {
    const row = document.createElement("tr");

    row.innerHTML = `
      <td>
        <div class="route-domain">
          <a href="https://${escapeHTML(route.domain)}" target="_blank" rel="noreferrer">${escapeHTML(route.domain)}</a>
          <small>${escapeHTML(route.cert_path || "certificate pending")}</small>
        </div>
      </td>
      <td>
        <div>${escapeHTML(route.target_ip)}:${escapeHTML(String(route.target_port))}</div>
        <small class="target-meta">${escapeHTML(route.key_path || "no private key yet")}</small>
      </td>
      <td><span class="tls-pill ${route.tls_ready ? "ready" : "pending"}">${route.tls_ready ? "Ready" : "Pending"}</span></td>
      <td>${escapeHTML(formatTimestamp(route.updated_at))}</td>
      <td>
        <div class="actions">
          <button class="link-button" type="button" data-action="edit" data-domain="${encodeURIComponent(route.domain)}">Edit</button>
          <button class="link-button danger" type="button" data-action="delete" data-domain="${encodeURIComponent(route.domain)}">Delete</button>
        </div>
      </td>
    `;

    row.querySelector('[data-action="edit"]').addEventListener("click", () => beginEdit(route.domain));
    row.querySelector('[data-action="delete"]').addEventListener("click", () => removeRoute(route.domain));
    elements.routesBody.appendChild(row);
  }
}

function beginEdit(domain) {
  const route = state.routes.find((item) => item.domain === domain);
  if (!route) {
    showNotice(`Route ${domain} not found in current list.`, "warning");
    return;
  }

  state.editingDomain = domain;
  elements.formTitle.textContent = `Edit ${domain}`;
  elements.submitButton.textContent = "Update Route";
  elements.cancelEditButton.classList.remove("hidden");
  elements.domain.readOnly = true;
  elements.domain.value = route.domain;
  elements.targetIP.value = route.target_ip;
  elements.targetPort.value = String(route.target_port);
  elements.domain.focus();
}

function resetForm() {
  state.editingDomain = null;
  elements.form.reset();
  elements.formTitle.textContent = "Create Route";
  elements.submitButton.textContent = "Create Route";
  elements.cancelEditButton.classList.add("hidden");
  elements.domain.readOnly = false;
}

async function removeRoute(domain) {
  const confirmed = window.confirm(`Delete route ${domain}?`);
  if (!confirmed) {
    return;
  }

  try {
    await requestJSON(`/routes/${encodeURIComponent(domain)}`, { method: "DELETE" });
    if (state.editingDomain === domain) {
      resetForm();
    }
    showNotice(`Deleted ${domain}.`, "success");
    await refreshAll(false);
  } catch (error) {
    showNotice(error.message, "error");
  }
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

function showNotice(message, tone) {
  elements.notice.className = `notice ${tone}`;
  elements.notice.textContent = message;
  elements.notice.classList.remove("hidden");
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
