import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { Globe, Lock, Plus, ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";

import { OrgHeader } from "@/components/OrgHeader";
import { Button } from "@/components/ui/button";
import { usePageTitle } from "@/lib/page-title";
import { type OrgMember, removeOrgMember, setOrgMemberPublic } from "@/lib/queries/org";
import { userProfileQuery } from "@/lib/queries/user";
import { subUrl } from "@/lib/url";
import { useOrgChange } from "@/lib/use-org-change";
import { useUserInfo } from "@/lib/use-user-info";

// OrgMembers lists the members of an organization, with their role and
// whether their membership is public.
export function OrgMembers({ org: orgName, members }: { org: string; members: OrgMember[] }) {
  const { t } = useTranslation();
  const { data: org } = useSuspenseQuery(userProfileQuery(orgName));
  usePageTitle(org.fullName || org.name);
  if (org.kind !== "organization") return null;

  return (
    <main className="flex-1">
      <OrgHeader org={org} activeTab="people" />
      <div className="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6">
        {org.viewerIsOwner ? (
          <div className="mb-4 flex justify-end">
            <Button asChild variant="outline" size="sm">
              <a href={subUrl(`/org/${org.name}/invitations/new`)}>
                <Plus className="size-4" aria-hidden />
                {t("org.invite_someone")}
              </a>
            </Button>
          </div>
        ) : null}
        <ul className="border-t border-(--color-border)">
          {members.map((member) => (
            <MemberRow key={member.name} org={org.name} member={member} viewerIsOwner={org.viewerIsOwner} />
          ))}
        </ul>
      </div>
    </main>
  );
}

function MemberRow({ org, member, viewerIsOwner }: { org: string; member: OrgMember; viewerIsOwner: boolean }) {
  const { t } = useTranslation();
  const user = useUserInfo();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const isSelf = user?.username === member.name;

  const visibility = useOrgChange(org, () => setOrgMemberPublic(org, member.name, !member.isPublic));
  const removal = useOrgChange(
    org,
    () => removeOrgMember(org, member.name),
    isSelf
      ? async () => {
          // The organization is gone from every page the viewer had loaded.
          await queryClient.invalidateQueries();
          await navigate({ to: "/" });
        }
      : undefined,
  );
  const pending = visibility.isPending || removal.isPending;

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-(--color-border) py-4">
      <div className="flex min-w-48 flex-1 items-center gap-3">
        <img
          src={member.avatarURL}
          alt=""
          className="size-10 shrink-0 rounded-full border border-(--color-border) object-cover"
        />
        <div className="min-w-0">
          <p className="flex flex-wrap items-baseline gap-x-2">
            <Link
              to="/$username"
              params={{ username: member.name }}
              search={{}}
              className="font-semibold wrap-anywhere text-(--color-primary) hover:underline"
            >
              {member.name}
            </Link>
            {member.fullName ? (
              <span className="text-sm wrap-anywhere text-(--color-muted-foreground)">{member.fullName}</span>
            ) : null}
          </p>
          <p className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-xs text-(--color-muted-foreground)">
            <span className="inline-flex items-center gap-1">
              {member.isOwner ? <ShieldCheck className="size-3.5" aria-hidden /> : null}
              {member.isOwner ? t("org.members.owner") : t("org.members.member")}
            </span>
            <span className="inline-flex items-center gap-1">
              {member.isPublic ? <Globe className="size-3.5" aria-hidden /> : <Lock className="size-3.5" aria-hidden />}
              {member.isPublic ? t("org.members.public") : t("org.members.private")}
            </span>
          </p>
        </div>
      </div>
      {isSelf || viewerIsOwner ? (
        <div className="flex shrink-0 flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            aria-label={`${member.isPublic ? t("org.members.public_helper") : t("org.members.private_helper")} ${member.name}`}
            disabled={pending}
            onClick={() => visibility.mutate()}
          >
            {member.isPublic ? t("org.members.public_helper") : t("org.members.private_helper")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            aria-label={isSelf ? undefined : `${t("org.members.remove")} ${member.name}`}
            className="text-(--color-destructive)"
            disabled={pending}
            onClick={() => removal.mutate()}
          >
            {isSelf ? t("org.members.leave") : t("org.members.remove")}
          </Button>
        </div>
      ) : null}
    </li>
  );
}
