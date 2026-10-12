import { useQuery } from '@tanstack/react-query';

import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';
import { PlanListSchema, type Plan } from '@/schemas/plan';

async function fetchPlans(): Promise<Plan[]> {
  const msg = await HttpUtil.get('/panel/api/plans', undefined, { silent: true });
  if (!msg?.success) throw new Error(msg?.msg || 'Failed to fetch plans');
  const validated = parseMsg(msg, PlanListSchema, 'plans');
  return Array.isArray(validated.obj) ? validated.obj : [];
}

export function usePlansQuery() {
  return useQuery({
    queryKey: keys.plans.list(),
    queryFn: fetchPlans,
  });
}
