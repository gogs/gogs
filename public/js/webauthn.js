"use strict";

window.gogsWebAuthn = window.gogsWebAuthn || {};

window.gogsWebAuthn.base64URLToBuffer = function(value) {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "===".slice((normalized.length + 3) % 4);
  const binary = atob(padded);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes.buffer;
};

window.gogsWebAuthn.bufferToBase64URL = function(buffer) {
  const bytes = new Uint8Array(buffer);
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
};

window.gogsWebAuthn.errorDetail = function(error) {
  if (!error) {
    return "";
  }

  const name = typeof error.name === "string" ? error.name.trim() : "";
  const message = typeof error.message === "string" ? error.message.trim() : "";
  if (name && message) {
    return name + ": " + message;
  }
  if (message) {
    return message;
  }
  if (name) {
    return name;
  }

  try {
    return String(error);
  } catch (_) {
    return "";
  }
};
