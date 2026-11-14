import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { toast } from "sonner";

import { userProfileQuery } from "@/lib/queries/user";

// useOrgChange runs a change to the members or teams of an organization, then
// reloads the page and the organization's counts, unless onSuccess takes the
// viewer elsewhere. Errors, e.g., removing the last owner, show as a toast with
// the server's localized message.
export function useOrgChange(org: string, change: () => Promise<void>, onSuccess?: () => Promise<void>) {
  const queryClient = useQueryClient();
  const router = useRouter();
  return useMutation({
    mutationFn: change,
    onSuccess: async () => {
      if (onSuccess) {
        await onSuccess();
        return;
      }
      await queryClient.invalidateQueries({ queryKey: userProfileQuery(org).queryKey });
      await router.invalidate();
    },
    onError: (err) => toast.error(err.message),
  });
}
