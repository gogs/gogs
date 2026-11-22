import { ArrowDown, ArrowUp, ChevronsUpDown } from "lucide-react";
import * as React from "react";

import { cn } from "@/lib/utils";

// Table does not scroll sideways. Hide the columns that do not fit a narrow
// card, e.g., with `hidden @xl:table-cell` on their cells inside a
// `@container` card, tell what they hold below the main column until they
// show, and let the main column wrap.
function Table({ className, ...props }: React.ComponentProps<"table">) {
  return <table className={cn("w-full caption-bottom text-sm", className)} {...props} />;
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return <thead className={cn("border-b border-(--color-border)", className)} {...props} />;
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return <tbody className={cn("[&_tr:last-child]:border-0", className)} {...props} />;
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      className={cn(
        "border-b border-(--color-border) transition-colors hover:bg-(--color-foreground)/3 data-[state=selected]:bg-(--color-foreground)/5",
        className,
      )}
      {...props}
    />
  );
}

function TableHead({ className, ...props }: React.ComponentProps<"th">) {
  return (
    <th
      scope="col"
      className={cn(
        "h-10 px-3 text-left align-middle text-xs font-medium whitespace-nowrap text-(--color-muted-foreground) first:pl-5 last:pr-5 sm:first:pl-6 sm:last:pr-6",
        className,
      )}
      {...props}
    />
  );
}

// TableSortHead heads a column that sorts the table when clicked. `sorted` is
// the direction the table is sorted in by this column, if it is. Numbers align
// right, with the arrow before the label, so the label stays over them.
function TableSortHead({
  sorted,
  onSort,
  align = "left",
  className,
  children,
  ...props
}: React.ComponentProps<"th"> & {
  sorted?: "ascending" | "descending";
  onSort: () => void;
  align?: "left" | "right";
}) {
  const Icon = sorted === "ascending" ? ArrowUp : sorted === "descending" ? ArrowDown : ChevronsUpDown;
  return (
    <TableHead aria-sort={sorted} className={cn(align === "right" && "text-right", className)} {...props}>
      <button
        type="button"
        onClick={onSort}
        className={cn(
          "-mx-1 inline-flex min-h-6 cursor-pointer items-center gap-1 rounded-sm px-1 font-medium outline-none hover:text-(--color-foreground) focus-visible:ring-1 focus-visible:ring-(--color-ring)",
          align === "right" && "flex-row-reverse",
          sorted && "text-(--color-foreground)",
        )}
      >
        {children}
        <Icon className="size-3.5 shrink-0" aria-hidden />
      </button>
    </TableHead>
  );
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  return (
    <td
      className={cn("px-3 py-2.5 align-middle first:pl-5 last:pr-5 sm:first:pl-6 sm:last:pr-6", className)}
      {...props}
    />
  );
}

function TableCaption({ className, ...props }: React.ComponentProps<"caption">) {
  return <caption className={cn("mt-4 text-sm text-(--color-muted-foreground)", className)} {...props} />;
}

export { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow, TableSortHead };
