import { useTranslation } from "react-i18next";

import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
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

// CommitAuthors shows the avatars of the author and the co-authors of a
// commit overlapping, then their names, like GitHub: "alice", "alice and
// carol", or "alice and 2 others" that lists the others on a click.
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
        <AuthorName author={author} className="truncate font-semibold" />
        {coAuthors.length === 1 ? (
          <>
            <span className="font-normal text-(--color-muted-foreground)">{t("repo.authors_and")}</span>
            <AuthorName author={coAuthors[0]} className="truncate font-semibold" />
          </>
        ) : coAuthors.length > 1 ? (
          <>
            <span className="font-normal text-(--color-muted-foreground)">{t("repo.authors_and")}</span>
            <Popover>
              <PopoverTrigger className="cursor-pointer font-semibold whitespace-nowrap hover:underline">
                {t("repo.authors_others", { count: coAuthors.length })}
              </PopoverTrigger>
              <PopoverContent align="start" className="flex w-64 flex-col p-1 text-sm">
                {people.map((person) => (
                  <span key={person.email} className="flex items-center gap-2 rounded-sm px-2 py-1.5">
                    <img src={person.avatarURL} alt="" className="size-5 shrink-0 rounded-full" />
                    <AuthorName author={person} className="min-w-0 truncate font-semibold" />
                  </span>
                ))}
              </PopoverContent>
            </Popover>
          </>
        ) : null}
      </span>
    </span>
  );
}
