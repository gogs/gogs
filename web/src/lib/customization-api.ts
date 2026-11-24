import { Link } from "@tanstack/react-router";
import htm from "htm";
import { t } from "i18next";
import * as React from "react";

import { Button } from "@/components/ui/button";
import { type Customization, SLOT_NAMES, type SlotName, type SlotProps } from "@/lib/customization";
import { subUrl } from "@/lib/url";
import { useUserInfo } from "@/lib/use-user-info";

// customizationAPI is what the app passes to the script of each part. Parts
// render with the React of the app, so hooks and its components work in them.
// `html` is htm bound to React, a JSX-like syntax that needs no build step.
const customizationAPI = {
  React,
  html: htm.bind(React.createElement),
  Link,
  Button,
  t,
  subUrl,
  useUser: useUserInfo,
};

// PART_TIMEOUT_MS is how long a part may take to load. A script that takes
// longer, e.g., one that awaits a server that does not answer, stays built-in,
// so it cannot hold back the other parts or the page.
const PART_TIMEOUT_MS = 3000;

function withTimeout<T>(load: Promise<T>, name: SlotName): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(
      () => reject(new Error(`The script of "${name}" did not load within ${PART_TIMEOUT_MS} ms.`)),
      PART_TIMEOUT_MS,
    );
  });
  return Promise.race([load, timeout]).finally(() => clearTimeout(timer));
}

let elementCount = 0;

// slotComponent turns the default export of the script of a part into its
// component. The export is either a subclass of HTMLElement, i.e., a web
// component, or a function that takes the customization API and returns a
// React component.
function slotComponent(name: SlotName, exported: unknown): React.ComponentType<SlotProps> {
  if (typeof exported !== "function") throw new TypeError(`The script of "${name}" has no default export.`);
  if (exported.prototype instanceof HTMLElement) {
    // Each load defines a tag of its own with a subclass of its own, since
    // neither a tag nor a class can be defined twice.
    const tag = `gogs-${name}-${++elementCount}`;
    customElements.define(tag, class extends (exported as CustomElementConstructor) {});
    return function CustomElement({ user }: SlotProps) {
      return React.createElement(tag, {
        authenticated: user ? "" : undefined,
        ref: (el: (HTMLElement & { user?: unknown }) | null) => {
          if (el) el.user = user;
        },
      });
    };
  }
  const component = (exported as (api: typeof customizationAPI) => unknown)(customizationAPI);
  if (typeof component !== "function") {
    throw new TypeError(`The default export of "${name}" must return a React component.`);
  }
  return component as React.ComponentType<SlotProps>;
}

export interface ImportResult {
  customization: Customization;
  // errors tells why the parts that failed to load did, which leaves them
  // built-in.
  errors: Partial<Record<SlotName, string>>;
}

// importParts imports the script of each part from its URL. A part that fails
// to load stays built-in, and the others still load.
export async function importParts(urls: Partial<Record<SlotName, string>>): Promise<ImportResult> {
  const result: ImportResult = { customization: { slots: {}, css: "" }, errors: {} };
  const names = SLOT_NAMES.filter((name) => urls[name]);
  const loaded = await Promise.allSettled(
    names.map(async (name) => {
      const mod = (await withTimeout(import(/* @vite-ignore */ urls[name] as string), name)) as Record<string, unknown>;
      return { component: slotComponent(name, mod.default), css: typeof mod.css === "string" ? mod.css : "" };
    }),
  );
  loaded.forEach((part, i) => {
    const name = names[i];
    if (part.status === "rejected") {
      result.errors[name] = String(part.reason);
      return;
    }
    result.customization.slots[name] = part.value.component;
    result.customization.css += part.value.css;
  });
  return result;
}

// importCustomization imports the published customization, a module that
// exports the URL of the script of each customized part.
export async function importCustomization(url: string): Promise<ImportResult> {
  const bundle = (await import(/* @vite-ignore */ url)) as Record<string, unknown>;
  const urls: Partial<Record<SlotName, string>> = {};
  for (const name of SLOT_NAMES) {
    if (typeof bundle[name] === "string") urls[name] = bundle[name];
  }
  return importParts(urls);
}

// importSources imports scripts from their source, e.g., drafts in the editor.
export function importSources(sources: Partial<Record<SlotName, string>>): Promise<ImportResult> {
  const urls: Partial<Record<SlotName, string>> = {};
  for (const name of SLOT_NAMES) {
    const source = sources[name];
    if (source?.trim()) urls[name] = `data:text/javascript;charset=utf-8,${encodeURIComponent(source)}`;
  }
  return importParts(urls);
}
