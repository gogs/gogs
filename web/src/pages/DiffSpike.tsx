import { parsePatchFiles } from "@pierre/diffs";
import { CodeView, type CodeViewItem } from "@pierre/diffs/react";

import { useTheme } from "@/lib/theme-context";

const SAMPLE_PATCH = `diff --git a/internal/route/repo/commit.go b/internal/route/repo/commit.go
index 1111111..2222222 100644
--- a/internal/route/repo/commit.go
+++ b/internal/route/repo/commit.go
@@ -16,7 +16,7 @@ import (
 )

 const (
-	COMMITS = "repo/commits"
+	COMMITS = "repo/commits_table"
 	DIFF    = "repo/diff/page"
 )

@@ -160,6 +160,9 @@ func Diff(c *context.Context) {
 	c.Data["Commit"] = commit
 	c.Data["Author"] = tryGetUserByEmail(c.Req.Context(), commit.Author.Email)
 	c.Data["Diff"] = diff
+	c.Data["Parents"] = parents
+	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
+	c.Data["SourcePath"] = path.Join(userName, repoName, "src", commitID)
 	c.Success(DIFF)
 }

diff --git a/web/src/pages/Hello.tsx b/web/src/pages/Hello.tsx
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/web/src/pages/Hello.tsx
@@ -0,0 +1,5 @@
+export function Hello() {
+  return <h1>hello, pierre/diffs</h1>;
+}
+
+export default Hello;
`;

const items: CodeViewItem[] = parsePatchFiles(SAMPLE_PATCH).flatMap((parsed, patchIndex) =>
  parsed.files.map<CodeViewItem>((fileDiff, fileIndex) => ({
    id: `${patchIndex}:${fileIndex}:${fileDiff.name}`,
    type: "diff",
    fileDiff,
  })),
);

export function DiffSpike() {
  const { theme } = useTheme();
  return (
    <main className="mx-auto w-full max-w-5xl px-4 py-8">
      <h1 className="mb-4 text-lg font-medium">@pierre/diffs spike</h1>
      <p className="mb-6 text-sm text-(--color-muted-foreground)">
        Throwaway page for evaluating the Pierre diff library against the Gogs shell. Remove once a real diff page
        exists.
      </p>
      <CodeView
        items={items}
        options={{
          theme: { light: "pierre-light", dark: "pierre-dark" },
          themeType: theme,
          diffStyle: "unified",
          stickyHeaders: true,
        }}
      />
    </main>
  );
}
