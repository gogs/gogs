import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { RepoCommitAuthor } from "@/lib/queries/repo";
import { routePath } from "@/lib/url";
import { cn } from "@/lib/utils";

function AuthorName({ author, className }: { author: RepoCommitAuthor; className?: string }) {
  return author.profileURL ? (
    <Link to={routePath(author.profileURL)} className={cn("hover:underline", className)}>
      {author.name}
    </Link>
  ) : (
    <span className={className}>{author.name}</span>
  );
}

// CommitAuthors shows the avatar and name of the author of a commit, and how
// many co-authors it has: "alice", "alice and 1 other", or "alice and 2
// others", where the count lists everyone on a click.
export function CommitAuthors({
  author,
  coAuthors = [],
  className,
}: {
  author: RepoCommitAuthor;
  coAuthors?: RepoCommitAuthor[];
  className?: string;
}) {
  const { t } = useTranslation();
  const people = [author, ...coAuthors];
  return (
    <span className={cn("inline-flex min-w-0 shrink-0 items-center gap-2", className)}>
      <img src={author.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
      <span className="flex min-w-0 items-center gap-1">
        <AuthorName author={author} className="truncate font-semibold" />
        {coAuthors.length > 0 ? (
          <>
            <span className="font-normal text-(--color-muted-foreground)">{t("repo.authors_and")}</span>
            <Popover>
              <PopoverTrigger className="cursor-pointer font-semibold whitespace-nowrap hover:underline">
                {coAuthors.length === 1
                  ? t("repo.authors_one_other")
                  : t("repo.authors_others", { count: coAuthors.length })}
              </PopoverTrigger>
              <PopoverContent
                align="start"
                collisionPadding={16}
                className="flex max-h-[min(20rem,var(--radix-popover-content-available-height))] w-64 flex-col p-1 text-sm"
              >
                <ScrollArea>
                  {people.map((person) => (
                    <span key={person.email} className="flex items-center gap-2 rounded-sm px-2 py-1.5">
                      <img src={person.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
                      <AuthorName author={person} className="min-w-0 truncate font-semibold" />
                    </span>
                  ))}
                </ScrollArea>
              </PopoverContent>
            </Popover>
          </>
        ) : null}
      </span>
    </span>
  );
}
