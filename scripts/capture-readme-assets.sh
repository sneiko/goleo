#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
CHROME_BIN=${CHROME_BIN:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}
CAPTURE_PORT=${CAPTURE_PORT:-7872}
FORM_PORT=${FORM_PORT:-${CAPTURE_PORT}}
CHAT_PORT=${CHAT_PORT:-$((CAPTURE_PORT + 1))}
ADAPTER_PORT=${ADAPTER_PORT:-$((CAPTURE_PORT + 2))}
TMP_DIR=$(mktemp -d)

FORM_PID=""
CHAT_PID=""
ADAPTER_PID=""

cleanup() {
  if [ -n "$FORM_PID" ]; then
    kill "$FORM_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$CHAT_PID" ]; then
    kill "$CHAT_PID" >/dev/null 2>&1 || true
  fi
  if [ -n "$ADAPTER_PID" ]; then
    kill "$ADAPTER_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}

wait_for_schema() {
  name=$1
  base_url=$2
  marker=$3
  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12; do
    if curl -fsS "${base_url}/api/schema" 2>/dev/null | grep -F -q "$marker"; then
      return 0
    fi
    sleep 0.5
  done
  echo "server did not start for ${name}" >&2
  return 1
}

capture() {
  name=$1
  width=$2
  height=$3
  url=$4
  output="${ROOT_DIR}/docs/assets/${name}.png"
  log_file="${TMP_DIR}/${name}.chrome.log"
  rm -f "${output}"
  "$CHROME_BIN" \
    --headless \
    --disable-gpu \
    --hide-scrollbars \
    --disable-background-networking \
    --disable-component-update \
    --disable-default-apps \
    --disable-sync \
    --metrics-recording-only \
    --no-first-run \
    --no-default-browser-check \
    --user-data-dir="${TMP_DIR}/${name}-profile" \
    --window-size="${width},${height}" \
    --virtual-time-budget=2000 \
    --screenshot="${output}" \
    "${url}" >"${log_file}" 2>&1 &
  chrome_pid=$!

  for attempt in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
    if [ -s "${output}" ]; then
      break
    fi
    if ! kill -0 "${chrome_pid}" >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done

  kill "${chrome_pid}" >/dev/null 2>&1 || true
  wait "${chrome_pid}" >/dev/null 2>&1 || true

  if [ ! -s "${output}" ]; then
    cat "${log_file}" >&2
    echo "failed to capture ${name}" >&2
    return 1
  fi
}

trap cleanup EXIT INT TERM

mkdir -p "${ROOT_DIR}/docs/assets"

(
  cd "${ROOT_DIR}"
  GOLEO_ADDR=":${FORM_PORT}" go run ./examples/showcase-form >"${TMP_DIR}/showcase-form.log" 2>&1
) &
FORM_PID=$!
FORM_BASE_URL="http://127.0.0.1:${FORM_PORT}"
wait_for_schema "showcase-form" "${FORM_BASE_URL}" "\"Launch summary\""
capture "readme-hero" 1440 1100 "${FORM_BASE_URL}/?demo=readme-hero"
capture "readme-components" 1440 1200 "${FORM_BASE_URL}/?demo=readme-components"
capture "readme-outputs" 1440 1100 "${FORM_BASE_URL}/?demo=readme-outputs"
capture "readme-mobile" 430 1180 "${FORM_BASE_URL}/?demo=readme-mobile"
kill "$FORM_PID" >/dev/null 2>&1 || true
wait "$FORM_PID" || true
FORM_PID=""

(
  cd "${ROOT_DIR}"
  GOLEO_ADDR=":${CHAT_PORT}" go run ./examples/showcase-chat >"${TMP_DIR}/showcase-chat.log" 2>&1
) &
CHAT_PID=$!
CHAT_BASE_URL="http://127.0.0.1:${CHAT_PORT}"
wait_for_schema "showcase-chat" "${CHAT_BASE_URL}" "\"kind\":\"chat\""
capture "readme-chat" 1440 1000 "${CHAT_BASE_URL}/?demo=readme-chat"
kill "$CHAT_PID" >/dev/null 2>&1 || true
wait "$CHAT_PID" || true
CHAT_PID=""

(
  cd "${ROOT_DIR}"
  GOLEO_ADDR=":${ADAPTER_PORT}" go run ./examples/showcase-adapters >"${TMP_DIR}/showcase-adapters.log" 2>&1
) &
ADAPTER_PID=$!
ADAPTER_BASE_URL="http://127.0.0.1:${ADAPTER_PORT}"
wait_for_schema "showcase-adapters" "${ADAPTER_BASE_URL}" "\"Backend metadata\""
capture "readme-adapters" 1440 1000 "${ADAPTER_BASE_URL}/?demo=readme-adapters"
kill "$ADAPTER_PID" >/dev/null 2>&1 || true
wait "$ADAPTER_PID" || true
ADAPTER_PID=""

file "${ROOT_DIR}"/docs/assets/readme-*.png
