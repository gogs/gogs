import { ChevronLeft, ChevronRight } from "lucide-react";
import * as React from "react";
import { DayPicker, getDefaultClassNames } from "react-day-picker";

import { buttonVariants } from "@/components/ui/button-variants";
import { cn } from "@/lib/utils";

function Calendar({ className, classNames, showOutsideDays = true, ...props }: React.ComponentProps<typeof DayPicker>) {
  const defaults = getDefaultClassNames();
  return (
    <DayPicker
      showOutsideDays={showOutsideDays}
      className={cn("p-3", className)}
      classNames={{
        root: cn("w-fit", defaults.root),
        months: cn("relative flex flex-col gap-4", defaults.months),
        month: cn("flex w-full flex-col gap-4", defaults.month),
        nav: cn("absolute inset-x-0 top-0 flex items-center justify-between", defaults.nav),
        button_previous: cn(buttonVariants({ variant: "ghost", size: "icon" }), "size-9", defaults.button_previous),
        button_next: cn(buttonVariants({ variant: "ghost", size: "icon" }), "size-9", defaults.button_next),
        month_caption: cn("flex h-9 items-center justify-center px-9", defaults.month_caption),
        caption_label: cn("text-sm font-medium", defaults.caption_label),
        month_grid: cn("w-full border-collapse", defaults.month_grid),
        weekdays: cn("flex", defaults.weekdays),
        weekday: cn("w-9 text-xs font-normal text-(--color-muted-foreground)", defaults.weekday),
        week: cn("mt-1 flex w-full", defaults.week),
        day: cn("size-9 p-0 text-center text-sm", defaults.day),
        day_button: cn(
          "size-9 cursor-pointer rounded-md outline-none hover:bg-(--color-surface) focus-visible:outline-2 focus-visible:outline-solid focus-visible:outline-(--color-ring)",
          defaults.day_button,
        ),
        selected: cn(
          "[&>button]:bg-(--color-primary) [&>button]:font-semibold [&>button]:text-(--color-primary-foreground) [&>button]:focus-visible:outline-offset-2 [&>button]:hover:bg-(--color-primary)",
          defaults.selected,
        ),
        today: cn("[&>button]:ring-1 [&>button]:ring-(--color-border)", defaults.today),
        outside: cn("text-(--color-muted-foreground)", defaults.outside),
        disabled: cn("opacity-50", defaults.disabled),
        hidden: cn("invisible", defaults.hidden),
        ...classNames,
      }}
      components={{
        Chevron: ({ orientation, className: chevronClassName }) => {
          const Icon = orientation === "left" ? ChevronLeft : ChevronRight;
          return <Icon className={cn("size-4", chevronClassName)} aria-hidden />;
        },
      }}
      {...props}
    />
  );
}

export { Calendar };
