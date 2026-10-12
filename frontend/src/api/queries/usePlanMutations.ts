import { useMutation, useQueryClient } from '@tanstack/react-query';

import { HttpUtil, Msg } from '@/utils';
import { keys } from '@/api/queryKeys';
import type { PlanFormValues } from '@/schemas/plan';
import type { Plan } from '@/schemas/plan';

type PlanPayload = Omit<Plan, 'id' | 'createdAt' | 'updatedAt'>;

function toPayload(values: PlanFormValues): PlanPayload {
  return {
    name: values.name.trim(),
    description: values.description.trim(),
    durationDays: values.durationDays,
    trafficLimitBytes: values.trafficLimitGB * 1024 ** 3,
    deviceLimit: values.deviceLimit,
    inboundIds: values.inboundIds,
    yooKassaPriceKopecks: Math.round(values.yooKassaPriceRubles * 100),
    plategaPriceKopecks: Math.round(values.plategaPriceRubles * 100),
    starsPrice: values.starsPrice,
    enabled: values.enabled,
    sortOrder: values.sortOrder,
  };
}

export function usePlanMutations() {
  const queryClient = useQueryClient();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: keys.plans.root() });

  const createMutation = useMutation({
    mutationFn: (values: PlanFormValues) =>
      HttpUtil.post<Plan>('/panel/api/plans', toPayload(values), {
        headers: { 'Content-Type': 'application/json' },
      }),
    onSuccess: (msg) => {
      if (msg?.success) void invalidate();
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, values }: { id: number; values: PlanFormValues }) =>
      HttpUtil.post<Plan>(`/panel/api/plans/${id}`, toPayload(values), {
        headers: { 'Content-Type': 'application/json' },
      }),
    onSuccess: (msg) => {
      if (msg?.success) void invalidate();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number): Promise<Msg<string>> => HttpUtil.delete(`/panel/api/plans/${id}`),
    onSuccess: (msg) => {
      if (msg?.success) void invalidate();
    },
  });

  return {
    create: (values: PlanFormValues) => createMutation.mutateAsync(values),
    update: (id: number, values: PlanFormValues) => updateMutation.mutateAsync({ id, values }),
    remove: (id: number) => deleteMutation.mutateAsync(id),
    saving: createMutation.isPending || updateMutation.isPending,
  };
}
