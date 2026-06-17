const appRoot = document.getElementById("app");

async function loadSchema() {
  const response = await fetch("/api/schema");
  if (!response.ok) {
    throw new Error(await response.text());
  }
  return response.json();
}

function renderComponent(component, value, onChange) {
  const props = component.props || {};
  const initialValue = value ?? props.default ?? "";
  const wrapper = document.createElement("label");
  wrapper.className = "field";
  if (props.visible === false) {
    wrapper.hidden = true;
  }

  const title = document.createElement("span");
  title.textContent = component.label;
  wrapper.appendChild(title);

  let input;
  switch (component.type) {
    case "number":
    case "slider":
      input = document.createElement("input");
      input.type = component.type === "slider" ? "range" : "number";
      applyInputAttributes(input, props, ["min", "max", "step"]);
      input.value = initialValue;
      if (props.default !== undefined) {
        onChange(Number(input.value));
      }
      input.addEventListener("input", () => onChange(Number(input.value)));
      break;
    case "checkbox":
      input = document.createElement("input");
      input.type = "checkbox";
      input.checked = Boolean(initialValue);
      onChange(input.checked);
      input.addEventListener("change", () => onChange(input.checked));
      break;
    case "dropdown":
      input = document.createElement("select");
      for (const choice of component.choices || []) {
        const option = document.createElement("option");
        option.value = choice;
        option.textContent = choice;
        input.appendChild(option);
      }
      input.value = initialValue;
      onChange(input.value);
      input.addEventListener("change", () => onChange(input.value));
      break;
    case "file":
    case "image":
      input = document.createElement("input");
      input.type = "file";
      applyInputAttributes(input, props, ["accept", "multiple"]);
      input.addEventListener("change", async () => {
        const file = input.files[0];
        if (!file) return;
        const form = new FormData();
        form.append("file", file);
        const response = await fetch("/api/upload", {
          method: "POST",
          body: form,
        });
        if (!response.ok) {
          throw new Error(await readErrorMessage(response));
        }
        onChange(await response.json());
      });
      break;
    default:
      input = document.createElement("textarea");
      applyInputAttributes(input, props, ["placeholder", "rows"]);
      input.value = initialValue;
      onChange(input.value);
      input.addEventListener("input", () => onChange(input.value));
      break;
  }

  if (props.disabled === true) {
    input.disabled = true;
  }

  wrapper.appendChild(input);
  return wrapper;
}

function applyInputAttributes(input, props, keys) {
  for (const key of keys) {
    if (props[key] !== undefined) {
      input[key] = props[key];
    }
  }
}

function renderOutput(component, value) {
  const section = document.createElement("section");
  section.className = "output";

  const title = document.createElement("h3");
  title.textContent = component.label;
  section.appendChild(title);

  const body = document.createElement("pre");
  if (component.type === "json" || typeof value === "object") {
    body.textContent = JSON.stringify(value ?? "", null, 2);
  } else {
    body.textContent = value ?? "";
  }
  section.appendChild(body);
  return section;
}

async function readErrorMessage(response) {
  try {
    const payload = await response.json();
    return payload.error?.message || response.statusText;
  } catch {
    return response.statusText;
  }
}

async function readServerSentEvents(response, onData) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const events = buffer.split("\n\n");
    buffer = events.pop() ?? "";
    for (const event of events) {
      handleServerSentEvent(event, onData);
    }
  }

  buffer += decoder.decode();
  if (buffer.trim() !== "") {
    handleServerSentEvent(buffer, onData);
  }
}

function handleServerSentEvent(rawEvent, onData) {
  const lines = rawEvent.split("\n");
  const event =
    lines
      .filter((line) => line.startsWith("event:"))
      .map((line) => line.slice("event:".length).trim())
      .at(-1) || "message";
  const data = lines
    .filter((line) => line.startsWith("data:"))
    .map((line) => line.slice("data:".length).trim())
    .join("\n")
    .replaceAll("\\n", "\n");

  if (event === "error") {
    throw new Error(data || "stream failed");
  }
  if (data !== "") {
    onData(data);
  }
}

function renderError(message) {
  const section = document.createElement("section");
  section.className = "output error";

  const title = document.createElement("h3");
  title.textContent = "Error";
  section.appendChild(title);

  const body = document.createElement("pre");
  body.textContent = message;
  section.appendChild(body);
  return section;
}

function renderInterface(iface) {
  const card = document.createElement("article");
  card.className = "interface";

  const title = document.createElement("h2");
  title.textContent = iface.kind === "chat" ? "Chat" : "Interface";
  card.appendChild(title);

  const values = iface.inputs.map(() => "");
  const outputs = document.createElement("div");
  outputs.className = "outputs";

  for (const [index, component] of iface.inputs.entries()) {
    card.appendChild(
      renderComponent(component, values[index], (value) => {
        values[index] = value;
      }),
    );
  }

  const button = document.createElement("button");
  button.textContent = iface.kind === "chat" ? "Send" : "Run";
  button.addEventListener("click", async () => {
    outputs.replaceChildren();
    try {
      const endpoint = iface.kind === "chat" ? "/api/stream" : "/api/predict";
      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ interface_id: iface.id, data: values }),
      });

      if (!response.ok) {
        throw new Error(await readErrorMessage(response));
      }

      if (iface.kind === "chat") {
        const output = renderOutput(iface.outputs[0], "");
        const body = output.querySelector("pre");
        outputs.appendChild(output);
        await readServerSentEvents(response, (chunk) => {
          body.textContent += chunk;
        });
        return;
      }

      const result = await response.json();
      for (const [index, component] of iface.outputs.entries()) {
        outputs.appendChild(renderOutput(component, result.data[index]));
      }
    } catch (error) {
      outputs.appendChild(renderError(error.message));
    }
  });
  card.appendChild(button);
  card.appendChild(outputs);
  return card;
}

loadSchema()
  .then((schema) => {
    const header = document.createElement("header");
    header.innerHTML = `<h1>Goleo</h1><p>AI demos from Go functions</p>`;
    appRoot.appendChild(header);
    for (const iface of schema.interfaces) {
      appRoot.appendChild(renderInterface(iface));
    }
  })
  .catch((error) => {
    appRoot.textContent = error.message;
  });
