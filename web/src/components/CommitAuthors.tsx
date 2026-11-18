import { useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import type { RepoCommitAuthor } from "@/lib/queries/repo";
import { cn } from "@/lib/utils";

function AuthorName({ author, className }: { author: RepoCommitAuthor; className?: string }) {
  return author.profileURL ? (
    <a href={author.profileURL} className={cn("hover:underline", className)}>
      {author.name}
    </a>
  ) : (
    <span className={className}>{author.name}</span>
  );
}

// CommitAuthors shows up to three avatars of the author and the co-authors of
// a commit overlapping, then their names, like GitHub: "alice", "alice and
// carol", or "3 people" that lists everyone on a click.
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
      <span className="flex shrink-0">
        {people.slice(0, 3).map((person, i) => (
          <img
            key={person.email}
            src={person.avatarURL}
            alt=""
            className={cn("size-5 rounded-full", i > 0 && "-ml-1.5 ring-2 ring-(--color-background)")}
          />
        ))}
      </span>
      <span className="flex min-w-0 items-center gap-1">
        {people.length > 2 ? (
          <Popover>
            <PopoverTrigger className="cursor-pointer font-semibold whitespace-nowrap hover:underline">
              {t("repo.authors_people", { count: people.length })}
            </PopoverTrigger>
            <PopoverContent
              align="start"
              avoidCollisions={false}
              collisionPadding={16}
              className="flex max-h-[min(20rem,var(--radix-popover-content-available-height))] w-64 flex-col p-1 text-sm"
            >
              <p className="px-2 py-1.5 text-xs text-(--color-muted-foreground)">
                {t("repo.authors_people", { count: people.length })}
              </p>
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
        ) : (
          <>
            <AuthorName author={author} className="truncate font-semibold" />
            {coAuthors.length === 1 && (
              <>
                <span className="font-normal text-(--color-muted-foreground)">{t("repo.authors_and")}</span>
                <AuthorName author={coAuthors[0]} className="truncate font-semibold" />
              </>
            )}
          </>
        )}
      </span>
    </span>
  );
}
