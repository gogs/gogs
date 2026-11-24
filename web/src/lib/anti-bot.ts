import { useEffect, useRef, useState } from "react";

import { type AltchaSolution, solveAltcha } from "@/lib/altcha";

// AntiBotVerification is the value of [auth] ANTI_BOT_VERIFICATION, which is
// empty when a form is not verified.
export type AntiBotVerification = "" | "altcha" | "captcha";

export type AltchaState = "idle" | "verifying" | "verified" | "error";

export interface AntiBot {
  verification: AntiBotVerification;
  // ready is whether the form can be submitted as far as the verification goes.
  ready: boolean;
  // payload returns what to submit as "antiBotPayload", or null when an altcha
  // solution is missing or expired.
  payload: () => string | null;
  // consume forgets an altcha solution about to be submitted, since the server
  // accepts each one only once.
  consume: () => void;
  // reset starts over, e.g., after a failed submission used up the captcha.
  reset: () => void;
  focus: () => void;

  captcha: string;
  setCaptcha: (value: string) => void;
  captchaRefresh: number;
  altchaState: AltchaState;
  verifyAltcha: () => void;
}

// useAntiBot keeps the state of the anti-bot verification of a form, which
// AntiBotField from @/components/AntiBotField renders.
export function useAntiBot(verification: AntiBotVerification): AntiBot {
  const [captcha, setCaptcha] = useState("");
  const [captchaRefresh, setCaptchaRefresh] = useState(0);
  const [altchaState, setAltchaState] = useState<AltchaState>("idle");
  const altchaSolution = useRef<AltchaSolution | null>(null);

  useEffect(() => {
    if (altchaState !== "verified" || !altchaSolution.current) return;
    // Start over once the solution expires, so a stale one is never sent.
    const timer = setTimeout(() => {
      altchaSolution.current = null;
      setAltchaState("idle");
    }, altchaSolution.current.expiresAt - Date.now());
    return () => clearTimeout(timer);
  }, [altchaState]);

  function verifyAltcha() {
    setAltchaState("verifying");
    solveAltcha().then(
      (solved) => {
        altchaSolution.current = solved;
        setAltchaState("verified");
      },
      () => {
        altchaSolution.current = null;
        setAltchaState("error");
      },
    );
  }

  return {
    verification,
    ready:
      verification === "" ||
      (verification === "captcha" && captcha.trim() !== "") ||
      (verification === "altcha" && altchaState === "verified"),
    payload: () => {
      if (verification !== "altcha") return captcha;
      const solution = altchaSolution.current;
      return solution && solution.expiresAt > Date.now() ? solution.payload : null;
    },
    consume: () => {
      altchaSolution.current = null;
    },
    reset: () => {
      if (verification === "altcha") {
        altchaSolution.current = null;
        setAltchaState("idle");
        return;
      }
      setCaptcha("");
      setCaptchaRefresh((value) => value + 1);
    },
    focus: () => document.getElementById(verification === "altcha" ? "altcha" : "captcha")?.focus(),
    captcha,
    setCaptcha,
    captchaRefresh,
    altchaState,
    verifyAltcha,
  };
}
