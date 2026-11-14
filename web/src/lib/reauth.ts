// Actions that change how to sign in to or access an account, such as adding
// an SSH key, need the viewer to have confirmed who they are in the last few
// minutes. The server answers them with `reauthRequired` otherwise, and
// confirmIdentity asks the viewer in the dialog of ReauthDialog. Requests that
// need it at the same time share one dialog.

let pending: { promise: Promise<boolean>; resolve: (confirmed: boolean) => void } | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const listener of listeners) listener();
}

// confirmIdentity asks the viewer to confirm who they are, and resolves to
// whether they did.
export function confirmIdentity(): Promise<boolean> {
  if (!pending) {
    let resolve!: (confirmed: boolean) => void;
    const promise = new Promise<boolean>((r) => (resolve = r));
    pending = { promise, resolve };
    emit();
  }
  return pending.promise;
}

// finishConfirmIdentity ends the confirmation the dialog shows.
export function finishConfirmIdentity(confirmed: boolean) {
  pending?.resolve(confirmed);
  pending = null;
  emit();
}

export function subscribeConfirmIdentity(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function isConfirmingIdentity() {
  return pending !== null;
}

// ReauthCancelledError is a request that was not sent again because the
// viewer did not confirm who they are.
export class ReauthCancelledError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ReauthCancelledError";
  }
}
