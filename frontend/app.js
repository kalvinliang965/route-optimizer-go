"use strict";

const limits = {
  maxStops: 10,
  maxTopK: 20,
  defaultTopK: 5
};

const demoStops = [
  {
    address: "Grand Central Terminal, 89 East 42nd Street, New York, NY 10017",
    label: "Depot"
  },
  {
    address: "United Nations Headquarters, New York, NY",
    label: "Client A"
  },
  {
    address: "The Plaza Hotel, New York, NY",
    label: "Client B"
  },
  {
    address: "Times Square, New York, NY 10036",
    label: "Client C"
  }
];

const elements = {
  serverStatus: document.querySelector("#server-status"),
  serverStatusLabel: document.querySelector("[data-status-label]"),
  maxStopsFact: document.querySelector("#max-stops-fact"),
  defaultTopKFact: document.querySelector("#default-top-k-fact"),
  stopsList: document.querySelector("#stops-list"),
  stopTemplate: document.querySelector("#stop-row-template"),
  stopCount: document.querySelector("#stop-count"),
  addStop: document.querySelector("#add-stop"),
  loadDemo: document.querySelector("#load-demo"),
  resolve: document.querySelector("#resolve-addresses"),
  geocodeMessage: document.querySelector("#geocode-message"),
  topK: document.querySelector("#top-k"),
  optimize: document.querySelector("#optimize-routes"),
  optimizeMessage: document.querySelector("#optimize-message"),
  resultsHeading: document.querySelector("#results-heading"),
  resultSummary: document.querySelector("#result-summary"),
  routeResults: document.querySelector("#route-results")
};

const state = {
  rows: [],
  busy: "",
  routes: [],
  nextRowKey: 1
};

function createRow(address = "", label = "") {
  return {
    key: `row-${state.nextRowKey++}`,
    address,
    label,
    inputMode: "address",
    latitude: "",
    longitude: "",
    coordinateSource: "entered",
    resolvedKey: "",
    lookupStop: null,
    stop: null,
    error: ""
  };
}

function addressKey(address) {
  return address.trim().replace(/\s+/g, " ").toLowerCase();
}

function isRowResolved(row) {
  if (row.inputMode === "coordinates") {
    const coordinates = parseCoordinates(row);
    return Boolean(coordinates && row.stop)
      && row.stop.lat === coordinates.lat
      && row.stop.lon === coordinates.lon;
  }
  return Boolean(row.stop) && row.resolvedKey === addressKey(row.address);
}

function parseCoordinates(row) {
  const latitudeText = String(row.latitude).trim();
  const longitudeText = String(row.longitude).trim();
  if (!latitudeText || !longitudeText) {
    return null;
  }

  const lat = Number(latitudeText);
  const lon = Number(longitudeText);
  if (!Number.isFinite(lat) || lat < -90 || lat > 90
    || !Number.isFinite(lon) || lon < -180 || lon > 180) {
    return null;
  }
  return { lat, lon };
}

function coordinateError(row) {
  const latitudeText = String(row.latitude).trim();
  const longitudeText = String(row.longitude).trim();
  if (!latitudeText && !longitudeText) {
    return "Enter latitude and longitude for this row.";
  }
  if (!latitudeText) {
    return "Enter a latitude between -90 and 90.";
  }
  if (!longitudeText) {
    return "Enter a longitude between -180 and 180.";
  }

  const lat = Number(latitudeText);
  if (!Number.isFinite(lat) || lat < -90 || lat > 90) {
    return "Latitude must be a number between -90 and 90.";
  }
  const lon = Number(longitudeText);
  if (!Number.isFinite(lon) || lon < -180 || lon > 180) {
    return "Longitude must be a number between -180 and 180.";
  }
  return "";
}

function effectiveStopName(row, index) {
  const providerName = row.lookupStop && row.resolvedKey === addressKey(row.address)
    ? row.lookupStop.name
    : "";
  return row.label.trim() || providerName || (index === 0 ? "Depot" : `Stop ${index + 1}`);
}

function syncCoordinateStop(row, index) {
  const coordinates = parseCoordinates(row);
  if (!coordinates) {
    row.stop = null;
    return false;
  }
  row.stop = {
    id: `stop-${index}`,
    name: effectiveStopName(row, index),
    lat: coordinates.lat,
    lon: coordinates.lon
  };
  return true;
}

function applyGeocodeResult(row, index, result) {
  if (result && result.stop && isFiniteCoordinate(result.stop.lat, result.stop.lon)) {
    const stop = {
      id: `stop-${index}`,
      name: String(result.stop.name || row.address.trim()),
      lat: Number(result.stop.lat),
      lon: Number(result.stop.lon)
    };
    row.lookupStop = stop;
    row.stop = stop;
    row.latitude = String(stop.lat);
    row.longitude = String(stop.lon);
    row.coordinateSource = "geocoded";
    row.inputMode = "coordinates";
    row.resolvedKey = addressKey(row.address);
    row.error = "";
    return true;
  }

  row.lookupStop = null;
  row.stop = null;
  row.latitude = "";
  row.longitude = "";
  row.coordinateSource = "entered";
  row.resolvedKey = "";
  row.error = result && result.error ? String(result.error) : "No location was returned.";
  return false;
}

function invalidateRoutes() {
  if (state.routes.length === 0) {
    return;
  }
  state.routes = [];
  setMessage(elements.optimizeMessage);
  renderRoutes();
}

function setMessage(element, message = "", kind = "") {
  element.textContent = message;
  element.classList.toggle("is-error", kind === "error");
  element.classList.toggle("is-success", kind === "success");
}

function renderRows() {
  const fragment = document.createDocumentFragment();

  state.rows.forEach((row, index) => {
    const article = elements.stopTemplate.content.firstElementChild.cloneNode(true);
    article.dataset.rowKey = row.key;

    const kind = article.querySelector('[data-role="kind"]');
    const kindHelp = article.querySelector('[data-role="kind-help"]');
    const inputMode = article.querySelector('[data-field="input-mode"]');
    const addressFields = article.querySelector('[data-role="address-fields"]');
    const addressInput = article.querySelector('[data-field="address"]');
    const labelInput = article.querySelector('[data-field="label"]');
    const coordinateFields = article.querySelector('[data-role="coordinate-fields"]');
    const latitudeInput = article.querySelector('[data-field="latitude"]');
    const longitudeInput = article.querySelector('[data-field="longitude"]');
    const removeButton = article.querySelector('[data-action="remove"]');

    kind.textContent = index === 0 ? "Depot" : `Stop ${index + 1}`;
    kindHelp.textContent = index === 0 ? "Start and finish" : "Delivery location";

    inputMode.value = row.inputMode;
    inputMode.disabled = Boolean(state.busy);
    inputMode.setAttribute("aria-label", `Input type for ${kind.textContent}`);
    addressFields.hidden = row.inputMode !== "address";
    coordinateFields.hidden = row.inputMode !== "coordinates";

    addressInput.value = row.address;
    addressInput.disabled = Boolean(state.busy);
    addressInput.id = `address-${row.key}`;
    addressInput.setAttribute("aria-label", `Address for ${kind.textContent}`);

    labelInput.value = row.label;
    labelInput.disabled = Boolean(state.busy);
    labelInput.id = `label-${row.key}`;
    labelInput.placeholder = index === 0 ? "Main warehouse" : "Customer name, purpose…";
    labelInput.setAttribute("aria-label", `Optional label for ${kind.textContent}`);

    latitudeInput.value = row.latitude;
    latitudeInput.disabled = Boolean(state.busy);
    latitudeInput.id = `latitude-${row.key}`;
    latitudeInput.setAttribute("aria-label", `Latitude for ${kind.textContent}`);

    longitudeInput.value = row.longitude;
    longitudeInput.disabled = Boolean(state.busy);
    longitudeInput.id = `longitude-${row.key}`;
    longitudeInput.setAttribute("aria-label", `Longitude for ${kind.textContent}`);

    removeButton.disabled = Boolean(state.busy) || state.rows.length <= 2;
    removeButton.setAttribute("aria-label", `Remove ${kind.textContent}`);

    addressInput.addEventListener("input", (event) => {
      const previousKey = addressKey(row.address);
      row.address = event.currentTarget.value;
      if (addressKey(row.address) !== previousKey || row.resolvedKey !== addressKey(row.address)) {
        row.stop = null;
        row.lookupStop = null;
        row.latitude = "";
        row.longitude = "";
        row.coordinateSource = "entered";
        row.resolvedKey = "";
        row.error = "";
        invalidateRoutes();
      }
      updateRowResolution(article, row);
      updateControls();
    });

    labelInput.addEventListener("input", (event) => {
      row.label = event.currentTarget.value;
      if (row.inputMode === "coordinates" && row.coordinateSource === "entered") {
        syncCoordinateStop(row, index);
      }
      if (state.routes.length > 0) {
        renderRoutes();
      }
    });

    inputMode.addEventListener("change", (event) => {
      const nextMode = event.currentTarget.value;
      row.inputMode = nextMode === "coordinates" ? "coordinates" : "address";
      row.error = "";
      if (row.inputMode === "coordinates") {
        const hasCoordinates = String(row.latitude).trim() || String(row.longitude).trim();
        if (!hasCoordinates && row.lookupStop && isFiniteCoordinate(row.lookupStop.lat, row.lookupStop.lon)) {
          row.latitude = String(row.lookupStop.lat);
          row.longitude = String(row.lookupStop.lon);
          row.coordinateSource = "geocoded";
        }
        syncCoordinateStop(row, index);
      } else {
        row.stop = null;
      }
      invalidateRoutes();
      renderRows();

      const field = row.inputMode === "coordinates"
        ? elements.stopsList.querySelector(`[data-row-key="${row.key}"] [data-field="latitude"]`)
        : elements.stopsList.querySelector(`[data-row-key="${row.key}"] [data-field="address"]`);
      if (field) {
        field.focus();
      }
    });

    const updateCoordinate = (field, value) => {
      row[field] = value;
      row.coordinateSource = "entered";
      row.error = "";
      syncCoordinateStop(row, index);
      invalidateRoutes();
      updateRowResolution(article, row);
      updateControls();
    };
    latitudeInput.addEventListener("input", (event) => {
      updateCoordinate("latitude", event.currentTarget.value);
    });
    longitudeInput.addEventListener("input", (event) => {
      updateCoordinate("longitude", event.currentTarget.value);
    });

    removeButton.addEventListener("click", () => {
      const currentIndex = state.rows.findIndex((candidate) => candidate.key === row.key);
      if (currentIndex < 0 || state.rows.length <= 2 || state.busy) {
        return;
      }
      state.rows.splice(currentIndex, 1);
      invalidateRoutes();
      renderRows();
      setMessage(elements.geocodeMessage, `Removed stop ${currentIndex + 1}.`);
      const nextIndex = Math.min(currentIndex, state.rows.length - 1);
      focusRowInput(state.rows[nextIndex]);
    });

    updateRowResolution(article, row);
    fragment.append(article);
  });

  elements.stopsList.replaceChildren(fragment);
  elements.stopsList.setAttribute("aria-busy", state.busy === "geocode" ? "true" : "false");
  updateControls();
}

function updateRowResolution(article, row) {
  const addressInput = article.querySelector('[data-field="address"]');
  const latitudeInput = article.querySelector('[data-field="latitude"]');
  const longitudeInput = article.querySelector('[data-field="longitude"]');
  const title = article.querySelector('[data-role="resolution-title"]');
  const detail = article.querySelector('[data-role="resolution-detail"]');
  const status = article.querySelector('[data-role="resolution"]');
  const errorID = `resolution-${row.key}`;

  status.id = errorID;
  article.classList.remove("is-resolved", "has-error");
  [addressInput, latitudeInput, longitudeInput].forEach((input) => {
    input.removeAttribute("aria-invalid");
    input.removeAttribute("aria-describedby");
  });

  if (row.inputMode === "address" && state.busy === "geocode") {
    title.textContent = "Resolving address";
    detail.textContent = "Cached locations return immediately; new locations may take longer.";
    return;
  }

  if (row.error) {
    article.classList.add("has-error");
    const invalidInputs = [];
    if (row.inputMode === "address") {
      invalidInputs.push(addressInput);
    }
    if (row.inputMode === "coordinates") {
      const lat = Number(String(row.latitude).trim());
      const lon = Number(String(row.longitude).trim());
      if (!String(row.latitude).trim() || !Number.isFinite(lat) || lat < -90 || lat > 90) {
        invalidInputs.push(latitudeInput);
      }
      if (!String(row.longitude).trim() || !Number.isFinite(lon) || lon < -180 || lon > 180) {
        invalidInputs.push(longitudeInput);
      }
    }
    invalidInputs.forEach((input) => {
      input.setAttribute("aria-invalid", "true");
      input.setAttribute("aria-describedby", errorID);
    });
    title.textContent = row.inputMode === "coordinates"
      ? "Coordinates need attention"
      : "Could not resolve this address";
    detail.textContent = row.error;
    return;
  }

  if (isRowResolved(row)) {
    article.classList.add("is-resolved");
    title.textContent = row.coordinateSource === "geocoded" && row.lookupStop
      ? row.lookupStop.name
      : "Coordinates ready";
    detail.textContent = `${formatCoordinate(row.stop.lat)}, ${formatCoordinate(row.stop.lon)}`;
    return;
  }

  if (row.inputMode === "coordinates") {
    title.textContent = "Coordinates required";
    detail.textContent = coordinateError(row);
    return;
  }

  title.textContent = row.address.trim() ? "Waiting to resolve" : "Address required";
  detail.textContent = row.address.trim()
    ? "Resolve the batch to verify this location."
    : "Enter an address before resolving the batch.";
}

function updateControls() {
  const busy = Boolean(state.busy);
  const allResolved = state.rows.length >= 2 && state.rows.every(isRowResolved);
  const topK = Number(elements.topK.value);
  const stopCountValid = state.rows.length <= limits.maxStops;
  const topKValid = Number.isInteger(topK) && topK >= 1 && topK <= limits.maxTopK;

  elements.stopCount.textContent = `${state.rows.length} of ${limits.maxStops} stops`;
  elements.addStop.disabled = busy || state.rows.length >= limits.maxStops;
  elements.loadDemo.disabled = busy;
  elements.resolve.disabled = busy || !stopCountValid;
  elements.resolve.classList.toggle("is-loading", state.busy === "geocode");
  elements.resolve.textContent = state.busy === "geocode" ? "Resolving locations…" : "Resolve locations";

  elements.topK.disabled = busy;
  elements.optimize.disabled = busy || !stopCountValid || !allResolved || !topKValid;
  elements.optimize.classList.toggle("is-loading", state.busy === "optimize");
  elements.optimize.textContent = state.busy === "optimize" ? "Calculating routes…" : "Calculate top routes";

  if (!topKValid && elements.topK.value !== "") {
    elements.topK.setAttribute("aria-invalid", "true");
  } else {
    elements.topK.removeAttribute("aria-invalid");
  }
}

function focusAddress(rowKey) {
  window.requestAnimationFrame(() => {
    const input = elements.stopsList.querySelector(`[data-row-key="${rowKey}"] [data-field="address"]`);
    if (input) {
      input.focus();
    }
  });
}

function focusRowInput(row) {
  window.requestAnimationFrame(() => {
    const field = row.inputMode === "coordinates" ? "latitude" : "address";
    const input = elements.stopsList.querySelector(
      `[data-row-key="${row.key}"] [data-field="${field}"]`
    );
    if (input) {
      input.focus();
    }
  });
}

function focusRowError(row) {
  window.requestAnimationFrame(() => {
    let field = "address";
    if (row.inputMode === "coordinates") {
      const lat = Number(String(row.latitude).trim());
      const latitudeInvalid = !String(row.latitude).trim()
        || !Number.isFinite(lat) || lat < -90 || lat > 90;
      field = latitudeInvalid ? "latitude" : "longitude";
    }
    const input = elements.stopsList.querySelector(
      `[data-row-key="${row.key}"] [data-field="${field}"]`
    );
    if (input) {
      input.focus();
    }
  });
}

function addStop() {
  if (state.busy || state.rows.length >= limits.maxStops) {
    return;
  }
  const row = createRow();
  state.rows.push(row);
  invalidateRoutes();
  renderRows();
  setMessage(elements.geocodeMessage, `Added stop ${state.rows.length}.`);
  focusAddress(row.key);
}

function loadDemo() {
  if (state.busy) {
    return;
  }
  const hasUserInput = state.rows.some((row) => row.address.trim() || row.label.trim()
    || String(row.latitude).trim() || String(row.longitude).trim());
  if (hasUserInput && !window.confirm("Replace the current stop list with the NYC demo?")) {
    return;
  }

  const availableDemoStops = demoStops.slice(0, limits.maxStops);
  state.rows = availableDemoStops.map((stop) => createRow(stop.address, stop.label));
  state.routes = [];
  setMessage(elements.geocodeMessage, `${availableDemoStops.length}-stop demo loaded. Resolve it when ready.`);
  setMessage(elements.optimizeMessage);
  renderRows();
  renderRoutes();
}

async function resolveAddresses() {
  if (state.busy) {
    return;
  }

  state.rows.forEach((row) => { row.error = ""; });
  const blankRows = state.rows.filter((row) => row.inputMode === "address" && !row.address.trim());
  if (blankRows.length > 0) {
    blankRows.forEach((row) => {
      row.error = "Enter an address for this row.";
    });
    renderRows();
    setMessage(
      elements.geocodeMessage,
      `${blankRows.length} ${blankRows.length === 1 ? "row needs" : "rows need"} an address.`,
      "error"
    );
    focusAddress(blankRows[0].key);
    return;
  }

  const lookupEntries = [];
  state.rows.forEach((row, index) => {
    if (row.inputMode === "coordinates") {
      if (!syncCoordinateStop(row, index)) {
        row.error = coordinateError(row);
      }
      return;
    }
    lookupEntries.push({ row, index });
  });

  state.busy = "geocode";
  const coordinateCount = state.rows.length - lookupEntries.length;
  setMessage(
    elements.geocodeMessage,
    lookupEntries.length > 0
      ? `Resolving ${lookupEntries.length} ${lookupEntries.length === 1 ? "address" : "addresses"}. ${coordinateCount > 0 ? `${coordinateCount} coordinate ${coordinateCount === 1 ? "row is" : "rows are"} already entered.` : "New public-provider lookups are intentionally paced."}`
      : "Checking coordinates. No address lookup is needed."
  );
  setMessage(elements.optimizeMessage);
  renderRows();

  try {
    if (lookupEntries.length > 0) {
      const payload = await requestJSON("/v1/geocode", {
        addresses: lookupEntries.map(({ row }) => row.address.trim())
      });
      if (!Array.isArray(payload.results) || payload.results.length !== lookupEntries.length) {
        throw new Error("The server returned an incomplete geocode result.");
      }

      const resultsByIndex = new Map();
      payload.results.forEach((result) => {
        const index = result && result.index;
        if (!Number.isInteger(index) || index < 0 || index >= lookupEntries.length || resultsByIndex.has(index)) {
          throw new Error("The server returned invalid geocode row indexes.");
        }
        resultsByIndex.set(index, result);
      });
      if (resultsByIndex.size !== lookupEntries.length) {
        throw new Error("The server did not return every geocode row.");
      }

      lookupEntries.forEach(({ row, index }, lookupIndex) => {
        const result = resultsByIndex.get(lookupIndex);
        applyGeocodeResult(row, index, result);
      });
    }

    invalidateRoutes();
    const failures = state.rows.filter((row) => !isRowResolved(row)).length;
    if (failures === 0) {
      setMessage(
        elements.geocodeMessage,
        coordinateCount > 0
          ? `All ${state.rows.length} stops are ready. ${coordinateCount} coordinate ${coordinateCount === 1 ? "row required" : "rows required"} no lookup.`
          : `Resolved all ${state.rows.length} stops.`,
        "success"
      );
    } else {
      setMessage(
        elements.geocodeMessage,
        `${failures} ${failures === 1 ? "row needs" : "rows need"} attention. Correct the highlighted values and try again.`,
        "error"
      );
    }
  } catch (error) {
    setMessage(elements.geocodeMessage, friendlyError(error), "error");
  } finally {
    state.busy = "";
    renderRows();
    const firstError = state.rows.find((row) => row.error);
    if (firstError) {
      focusRowError(firstError);
    }
  }
}

async function optimizeRoutes() {
  if (state.busy || state.rows.length < 2 || !state.rows.every(isRowResolved)) {
    setMessage(elements.optimizeMessage, "Resolve each address or enter valid coordinates first.", "error");
    return;
  }

  const topK = Number(elements.topK.value);
  if (!Number.isInteger(topK) || topK < 1 || topK > limits.maxTopK) {
    setMessage(elements.optimizeMessage, `Choose a whole number from 1 to ${limits.maxTopK}.`, "error");
    elements.topK.focus();
    return;
  }

  state.busy = "optimize";
  setMessage(elements.optimizeMessage, "Building the duration matrix and ranking round trips.");
  updateControls();
  elements.routeResults.setAttribute("aria-busy", "true");

  try {
    const stops = state.rows.map((row, index) => (
      {
        id: `stop-${index}`,
        name: effectiveStopName(row, index),
        lat: row.stop.lat,
        lon: row.stop.lon
      }
    ));
    const payload = await requestJSON("/v1/optimize", {
      stops,
      top_k: topK
    });
    if (!Array.isArray(payload.routes)) {
      throw new Error("The server returned an invalid route result.");
    }

    state.routes = payload.routes;
    renderRoutes();
    const count = state.routes.length;
    setMessage(
      elements.optimizeMessage,
      `Calculated ${count} ${count === 1 ? "route" : "routes"}.`,
      "success"
    );
    focusResults();
  } catch (error) {
    state.routes = [];
    renderRoutes();
    setMessage(elements.optimizeMessage, friendlyError(error), "error");
  } finally {
    state.busy = "";
    elements.routeResults.setAttribute("aria-busy", "false");
    updateControls();
  }
}

function renderRoutes() {
  elements.routeResults.replaceChildren();
  elements.resultSummary.textContent = "";

  if (state.routes.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-results";

    const routeGraphic = document.createElement("div");
    routeGraphic.className = "empty-route";
    routeGraphic.setAttribute("aria-hidden", "true");
    routeGraphic.append(document.createElement("span"), document.createElement("span"), document.createElement("span"));

    const heading = document.createElement("h3");
    heading.textContent = "Your routes will appear here.";
    const copy = document.createElement("p");
    copy.textContent = "Resolve the stop list, choose how many options you want, and calculate.";
    empty.append(routeGraphic, heading, copy);
    elements.routeResults.append(empty);
    return;
  }

  elements.resultSummary.textContent = `${state.routes.length} ranked ${state.routes.length === 1 ? "option" : "options"}`;
  const fragment = document.createDocumentFragment();
  state.routes.forEach((route, routeIndex) => {
    fragment.append(buildRouteCard(route, routeIndex));
  });
  elements.routeResults.append(fragment);
}

function buildRouteCard(route, routeIndex) {
  const article = document.createElement("article");
  article.className = "route-card";

  const rank = document.createElement("div");
  rank.className = "route-rank";
  const rankLabel = document.createElement("span");
  rankLabel.textContent = "Rank";
  const rankValue = document.createElement("strong");
  rankValue.textContent = `#${Number(route.rank) || routeIndex + 1}`;
  const duration = document.createElement("p");
  duration.className = "route-duration";
  duration.textContent = formatDuration(Number(route.duration_seconds));
  rank.append(rankLabel, rankValue, duration);

  const routeBody = document.createElement("div");
  const routeStops = document.createElement("div");
  routeStops.className = "route-stops";
  const addresses = document.createElement("ol");
  addresses.className = "route-addresses";

  const path = Array.isArray(route.path) ? route.path : [];
  const orderedStops = Array.isArray(route.ordered_stops) ? route.ordered_stops : [];
  path.forEach((stopIndex, sequenceIndex) => {
    if (sequenceIndex > 0) {
      const arrow = document.createElement("span");
      arrow.className = "route-arrow";
      arrow.setAttribute("aria-hidden", "true");
      arrow.textContent = "→";
      routeStops.append(arrow);
    }

    const row = state.rows[Number(stopIndex)];
    const routeStop = document.createElement("span");
    routeStop.className = "route-stop";
    const label = document.createElement("span");
    label.className = "route-stop-label";
    label.textContent = row ? displayLabel(row, Number(stopIndex)) : `Stop ${Number(stopIndex) + 1}`;
    routeStop.append(label);
    routeStops.append(routeStop);

    const address = document.createElement("li");
    const orderedStop = orderedStops[sequenceIndex];
    address.textContent = orderedStop && orderedStop.name
      ? String(orderedStop.name)
      : row && row.stop ? row.stop.name : label.textContent;
    addresses.append(address);
  });

  routeBody.append(routeStops, addresses);
  article.append(rank, routeBody);

  const mapsURL = safeMapsURL(route.directions_url);
  if (mapsURL) {
    const mapsLink = document.createElement("a");
    mapsLink.className = "maps-link";
    mapsLink.href = mapsURL;
    mapsLink.target = "_blank";
    mapsLink.rel = "noopener noreferrer";
    mapsLink.textContent = "Open in Google Maps ↗";
    mapsLink.setAttribute("aria-label", `Open route ${Number(route.rank) || routeIndex + 1} in Google Maps`);
    article.append(mapsLink);
  }

  return article;
}

function displayLabel(row, index) {
  const label = row.label.trim();
  if (label) {
    return label;
  }
  return index === 0 ? "Depot" : `Stop ${index + 1}`;
}

function safeMapsURL(rawURL) {
  if (typeof rawURL !== "string" || !rawURL) {
    return "";
  }
  try {
    const parsed = new URL(rawURL);
    if (parsed.protocol !== "https:" || parsed.hostname !== "www.google.com" || !parsed.pathname.startsWith("/maps/")) {
      return "";
    }
    return parsed.toString();
  } catch {
    return "";
  }
}

function formatDuration(seconds) {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "Duration unavailable";
  }
  const totalMinutes = Math.max(0, Math.round(seconds / 60));
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours === 0) {
    return `${minutes} min estimated`;
  }
  if (minutes === 0) {
    return `${hours} hr estimated`;
  }
  return `${hours} hr ${minutes} min estimated`;
}

function formatCoordinate(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number.toFixed(5) : "unknown";
}

function isFiniteCoordinate(lat, lon) {
  const latitude = Number(lat);
  const longitude = Number(lon);
  return Number.isFinite(latitude) && Number.isFinite(longitude)
    && latitude >= -90 && latitude <= 90
    && longitude >= -180 && longitude <= 180;
}

async function requestJSON(path, body) {
  let response;
  try {
    response = await fetch(path, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    });
  } catch {
    throw new Error("Could not reach the route server. Check that it is running and try again.");
  }

  let payload;
  try {
    payload = await response.json();
  } catch {
    throw new Error(`The server returned an unreadable response (${response.status}).`);
  }
  if (!response.ok) {
    throw new Error(payload && payload.error ? String(payload.error) : `Request failed (${response.status}).`);
  }
  return payload;
}

function friendlyError(error) {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return "Something went wrong. Please try again.";
}

function focusResults() {
  elements.resultsHeading.setAttribute("tabindex", "-1");
  elements.resultsHeading.focus({ preventScroll: true });
  const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  elements.resultsHeading.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "start" });
  elements.resultsHeading.addEventListener("blur", () => {
    elements.resultsHeading.removeAttribute("tabindex");
  }, { once: true });
}

async function checkHealth() {
  try {
    const response = await fetch("/healthz", { headers: { "Accept": "application/json" } });
    if (!response.ok) {
      throw new Error("unhealthy");
    }
    elements.serverStatus.classList.add("is-online");
    elements.serverStatus.classList.remove("is-offline");
    elements.serverStatusLabel.textContent = "Server online";
  } catch {
    elements.serverStatus.classList.add("is-offline");
    elements.serverStatus.classList.remove("is-online");
    elements.serverStatusLabel.textContent = "Server unavailable";
  }
}

async function loadLimits() {
  try {
    const response = await fetch("/v1/config", { headers: { "Accept": "application/json" } });
    if (!response.ok) {
      throw new Error("configuration unavailable");
    }
    const payload = await response.json();
    const maxStops = Number(payload.max_stops);
    const maxTopK = Number(payload.max_top_k);
    const defaultTopK = Number(payload.default_top_k);
    if (!Number.isInteger(maxStops) || maxStops < 1
      || !Number.isInteger(maxTopK) || maxTopK < 1
      || !Number.isInteger(defaultTopK) || defaultTopK < 1 || defaultTopK > maxTopK) {
      throw new Error("configuration invalid");
    }

    limits.maxStops = maxStops;
    limits.maxTopK = maxTopK;
    limits.defaultTopK = defaultTopK;
    elements.topK.max = String(maxTopK);
    if (elements.topK.dataset.edited !== "true") {
      elements.topK.value = String(defaultTopK);
    }
    elements.maxStopsFact.textContent = `Up to ${maxStops} ${maxStops === 1 ? "stop" : "stops"}`;
    elements.defaultTopKFact.textContent = `Top ${defaultTopK} by default`;

    if (state.rows.length > maxStops) {
      setMessage(
        elements.geocodeMessage,
        `This server allows ${maxStops} ${maxStops === 1 ? "stop" : "stops"}; remove extra rows to continue.`,
        "error"
      );
    }
    updateControls();
  } catch {
    // The built-in defaults keep the static page usable if metadata cannot be loaded.
  }
}

elements.addStop.addEventListener("click", addStop);
elements.loadDemo.addEventListener("click", loadDemo);
elements.resolve.addEventListener("click", resolveAddresses);
elements.optimize.addEventListener("click", optimizeRoutes);
elements.topK.addEventListener("input", () => {
  elements.topK.dataset.edited = "true";
  setMessage(elements.optimizeMessage);
  invalidateRoutes();
  updateControls();
});

state.rows.push(createRow(), createRow());
renderRows();
renderRoutes();
checkHealth();
loadLimits();
