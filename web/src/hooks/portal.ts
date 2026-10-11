import { createContext, useContext } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { Account, Project } from "@/lib/types";

export const PortalContext = createContext<{
  account: Account;
  projects: Project[];
  project?: Project;
  select: (id: string) => void;
} | null>(null);
export function usePortal() {
  const value = useContext(PortalContext);
  if (!value) throw new Error("Portal context unavailable");
  return value;
}
export function useResource<T>(
  resource: string,
  query = "",
): {
  data: T | undefined;
  error: Error | null;
  isPending: boolean;
  refetch: () => Promise<unknown>;
} {
  const { project } = usePortal();
  return useQuery<T, Error>({
    queryKey: ["project", project?.id, resource, query],
    queryFn: () => api<T>(`/projects/${project!.id}/${resource}${query}`),
    enabled: Boolean(project),
    refetchInterval:
      resource === "dashboard" ||
      resource === "transactions" ||
      resource === "webhook-jobs"
        ? 10000
        : false,
  });
}
export function useTask() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      path,
      method = "POST",
      body,
    }: {
      path: string;
      method?: string;
      body?: unknown;
    }) => api<unknown>(path, method, body),
    onSuccess: () => {
      client.invalidateQueries();
      toast.success("Saved");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}
