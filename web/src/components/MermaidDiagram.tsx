import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { CodeBlock } from "@/components/CodeBlock";
import { resolveTheme, useTheme } from "@/lib/theme-context";

// Accent colors of Pierre Theme's "Light" and "Dark" variants
// (https://github.com/pierrecomputer/theme), in the order Mermaid gives them
// to pie slices, git branches and other series: blue, green, magenta, cyan,
// red and yellow.
const accents = {
  light: ["#1a85d4", "#18a46c", "#bd2e90", "#1ca1c7", "#d52c36", "#d5a910"],
  dark: ["#009fff", "#0dbe4e", "#e130ac", "#08c0ef", "#ff2e3f", "#ffca00"],
};

// pageTheme makes diagrams use the colors of the rest of the page. Mermaid
// derives more shades from these, so they must be colors, not var() references.
function pageTheme(theme: "light" | "dark") {
  const style = getComputedStyle(document.documentElement);
  const token = (name: string) => style.getPropertyValue(`--color-${name}`).trim();
  const background = token("background");
  const surface = token("surface");
  const foreground = token("foreground");
  const muted = token("muted-foreground");
  const border = token("border");
  const onAccent = token("primary-foreground");

  const series: Record<string, string> = {};
  accents[theme].forEach((color, i) => {
    series[`pie${i + 1}`] = color;
    series[`cScale${i}`] = color;
    series[`cScaleLabel${i}`] = onAccent;
    // The neo themes draw the first git branch in the node border color and
    // give the series colors to the branches after it.
    series[`git${i + 1}`] = color;
  });

  const themeVariables = {
    fontFamily: style.getPropertyValue("--font-sans"),
    // Flat shapes with rounded corners, as with cards on the page.
    useGradient: false,
    dropShadow: "none",
    radius: parseFloat(style.getPropertyValue("--radius")) * parseFloat(style.fontSize),

    background,
    primaryColor: surface,
    mainBkg: surface,
    nodeBkg: surface,
    primaryTextColor: foreground,
    nodeTextColor: foreground,
    textColor: foreground,
    titleColor: foreground,
    // The neo themes also draw the main branch and commit labels of git graphs
    // in this color, so it must be readable as text.
    primaryBorderColor: muted,
    nodeBorder: muted,
    lineColor: muted,
    arrowheadColor: muted,
    defaultLinkColor: muted,
    edgeLabelBackground: background,
    clusterBkg: background,
    clusterBorder: border,
    noteBkgColor: background,
    noteTextColor: foreground,
    noteBorderColor: border,

    actorBkg: surface,
    actorBorder: border,
    actorTextColor: foreground,
    actorLineColor: border,
    signalColor: muted,
    signalTextColor: foreground,
    labelBoxBkgColor: surface,
    labelBoxBorderColor: border,
    labelTextColor: foreground,
    loopTextColor: foreground,
    activationBkgColor: surface,
    activationBorderColor: border,

    pieOpacity: "1",
    pieStrokeColor: background,
    pieOuterStrokeColor: background,
    pieSectionTextColor: onAccent,
    pieTitleTextColor: foreground,
    pieLegendTextColor: foreground,
    tagLabelColor: foreground,
    ...series,
  };
  // Without the gradient, the neo themes leave git branch labels unfilled,
  // which draws them black.
  const themeCSS = `.branchLabelBkg { fill: ${surface}; stroke: ${border}; } .branch-label { fill: ${foreground}; }`;
  return { themeVariables, themeCSS };
}

// Mermaid is large, so it is only loaded on pages that have a diagram.
const loadMermaid = () => import("mermaid").then((m) => m.default);

// Mermaid keeps one configuration for the page, so concurrent renders with
// different themes would interfere. They run one after another instead.
let queue: Promise<unknown> = Promise.resolve();

function renderDiagram(id: string, code: string, theme: "light" | "dark"): Promise<string> {
  const next = queue.then(async () => {
    const mermaid = await loadMermaid();
    mermaid.initialize({
      startOnLoad: false,
      // Labels must not run scripts or link anywhere, because anyone who can
      // write Markdown writes them.
      securityLevel: "strict",
      // A diagram that fails to draw is shown as code instead of Mermaid's
      // error graphic, which it would add to the end of the page.
      suppressErrorRendering: true,
      look: "neo",
      theme: theme === "dark" ? "neo-dark" : "neo",
      ...pageTheme(theme),
    });
    const { svg } = await mermaid.render(id, code);
    return svg;
  });
  queue = next.catch(() => {});
  return next;
}

// MermaidDiagram draws a Mermaid code block as a diagram, or shows its code
// when the diagram cannot be drawn.
export function MermaidDiagram({ code }: { code: string }) {
  const { t } = useTranslation();
  const { theme } = useTheme();
  const resolved = resolveTheme(theme);
  // Mermaid uses the id for elements it adds to the document while drawing,
  // which must be valid in a CSS selector.
  const id = `mermaid-${useId().replace(/[^\w-]/g, "")}`;
  const [result, setResult] = useState<{ svg?: string; failed?: boolean }>({});

  useEffect(() => {
    let current = true;
    renderDiagram(id, code, resolved).then(
      (svg) => current && setResult({ svg }),
      () => current && setResult({ failed: true }),
    );
    return () => {
      current = false;
    };
  }, [id, code, resolved]);

  if (result.failed) {
    return <CodeBlock code={code} lang="text" />;
  }
  return (
    <div
      role="figure"
      aria-label={t("mermaid_diagram")}
      aria-busy={!result.svg}
      className="mermaid-diagram overflow-x-auto"
      // Mermaid sanitizes the SVG it draws in strict mode.
      dangerouslySetInnerHTML={{ __html: result.svg ?? "" }}
    />
  );
}
