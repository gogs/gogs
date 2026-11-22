import { subUrl } from "./url";

interface AltchaChallenge {
  parameters: { expiresAt: number } & Record<string, unknown>;
  signature: string;
}

interface WorkerMessage {
  tried?: number;
  counter?: number;
  derivedKey?: string;
  error?: string;
}

// AltchaSolution is the payload the server verifies, and when its challenge
// expires by the browser's clock.
export interface AltchaSolution {
  payload: string;
  expiresAt: number;
}

// solveAltcha fetches a proof-of-work challenge from the server and solves it
// with one Web Worker per CPU core, up to 8, each trying every n-th counter.
export async function solveAltcha(): Promise<AltchaSolution> {
  const res = await fetch(subUrl("/-/altcha/challenge.json"), {
    cache: "no-store",
    credentials: "same-origin",
  });
  if (!res.ok) throw new Error(`challenge: HTTP ${res.status}`);
  // Measure the lifetime against the server's clock, so a browser clock that is
  // off does not matter.
  const serverNow = Date.parse(res.headers.get("Date") ?? "") || Date.now();
  const offset = Date.now() - serverNow;
  const challenge = (await res.json()) as AltchaChallenge;

  const n = Math.min(8, Math.max(1, navigator.hardwareConcurrency || 1));
  const workers: Worker[] = [];
  const stop = () => workers.forEach((w) => w.terminate());
  try {
    const solution = await new Promise<{ counter: number; derivedKey: string }>((resolve, reject) => {
      for (let i = 0; i < n; i++) {
        const w = new Worker(subUrl("/-/altcha/worker.js"));
        w.onmessage = (e: MessageEvent<WorkerMessage>) => {
          if (e.data.error) reject(new Error(e.data.error));
          else if (e.data.derivedKey !== undefined && e.data.counter !== undefined) {
            resolve({ counter: e.data.counter, derivedKey: e.data.derivedKey });
          }
        };
        w.onerror = (e) => reject(new Error(e.message || "worker failed"));
        workers.push(w);
        w.postMessage({ parameters: challenge.parameters, start: i, step: n });
      }
    });
    return {
      payload: btoa(JSON.stringify({ challenge, solution })),
      // Leave a margin so a solution is never sent just before it expires.
      expiresAt: challenge.parameters.expiresAt * 1000 + offset - 10_000,
    };
  } finally {
    stop();
  }
}
