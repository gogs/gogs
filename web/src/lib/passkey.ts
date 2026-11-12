// Helpers for the WebAuthn ceremonies of passkeys. The server sends options
// and reads credentials as JSON, with binary values in base64url, which the
// browser API takes and returns as ArrayBuffers.

interface CredentialDescriptorJSON {
  type: "public-key";
  id: string;
  transports?: AuthenticatorTransport[];
}

export interface CreationOptionsJSON {
  publicKey: Omit<PublicKeyCredentialCreationOptions, "challenge" | "user" | "excludeCredentials"> & {
    challenge: string;
    user: { id: string; name: string; displayName: string };
    excludeCredentials?: CredentialDescriptorJSON[];
  };
}

export interface RequestOptionsJSON {
  publicKey: Omit<PublicKeyCredentialRequestOptions, "challenge" | "allowCredentials"> & {
    challenge: string;
    allowCredentials?: CredentialDescriptorJSON[];
  };
}

export function passkeysSupported() {
  return typeof window.PublicKeyCredential === "function" && "credentials" in navigator;
}

function fromBase64URL(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, "+").replace(/_/g, "/");
  const binary = atob(base64.padEnd(base64.length + ((4 - (base64.length % 4)) % 4), "="));
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

function toBase64URL(buffer: ArrayBuffer): string {
  let binary = "";
  for (const byte of new Uint8Array(buffer)) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function toDescriptors(list?: CredentialDescriptorJSON[]): PublicKeyCredentialDescriptor[] | undefined {
  return list?.map((c) => ({ ...c, id: fromBase64URL(c.id) }));
}

// createPasskey asks the browser to create a passkey with the options of the
// server, and returns it as the server reads it.
export async function createPasskey({ publicKey }: CreationOptionsJSON) {
  const credential = (await navigator.credentials.create({
    publicKey: {
      ...publicKey,
      challenge: fromBase64URL(publicKey.challenge),
      user: { ...publicKey.user, id: fromBase64URL(publicKey.user.id) },
      excludeCredentials: toDescriptors(publicKey.excludeCredentials),
    },
  })) as PublicKeyCredential;
  const response = credential.response as AuthenticatorAttestationResponse;
  return {
    id: credential.id,
    rawId: toBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: toBase64URL(response.clientDataJSON),
      attestationObject: toBase64URL(response.attestationObject),
      transports: response.getTransports(),
    },
  };
}

// getPasskey asks the browser for a passkey to answer the challenge of the
// server with, and returns the answer as the server reads it. The signal
// cancels the request, which is then rejected with its reason.
export async function getPasskey({ publicKey }: RequestOptionsJSON, signal: AbortSignal) {
  const credential = (await navigator.credentials.get({
    publicKey: {
      ...publicKey,
      challenge: fromBase64URL(publicKey.challenge),
      allowCredentials: toDescriptors(publicKey.allowCredentials),
      // The caller times the request out with the signal, so it can tell a
      // timeout from the visitor cancelling, which browsers report the same way.
      timeout: undefined,
    },
    signal,
  })) as PublicKeyCredential;
  const response = credential.response as AuthenticatorAssertionResponse;
  return {
    id: credential.id,
    rawId: toBase64URL(credential.rawId),
    type: credential.type,
    authenticatorAttachment: credential.authenticatorAttachment ?? undefined,
    clientExtensionResults: credential.getClientExtensionResults(),
    response: {
      clientDataJSON: toBase64URL(response.clientDataJSON),
      authenticatorData: toBase64URL(response.authenticatorData),
      signature: toBase64URL(response.signature),
      userHandle: response.userHandle ? toBase64URL(response.userHandle) : undefined,
    },
  };
}
