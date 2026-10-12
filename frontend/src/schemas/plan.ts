import { z } from 'zod';

export const PlanSchema = z.object({
  id: z.number().int().positive(),
  name: z.string(),
  description: z.string(),
  durationDays: z.number().int().positive(),
  trafficLimitBytes: z.number().int().nonnegative(),
  deviceLimit: z.number().int().nonnegative(),
  inboundIds: z.array(z.number().int().positive()),
  yooKassaPriceKopecks: z.number().int().positive(),
  plategaPriceKopecks: z.number().int().positive(),
  starsPrice: z.number().int().positive(),
  enabled: z.boolean(),
  sortOrder: z.number().int().nonnegative(),
  createdAt: z.number().int(),
  updatedAt: z.number().int(),
});

export const PlanListSchema = z.array(PlanSchema);

export type Plan = z.infer<typeof PlanSchema>;

export const PlanFormSchema = z
  .object({
    name: z.string().trim().min(1, 'pages.plans.nameRequired').max(120, 'pages.plans.nameRequired'),
    description: z.string().max(2000, 'pages.plans.descriptionTooLong'),
    durationDays: z.number().int().min(1, 'pages.plans.durationRequired').max(3650),
    trafficLimitGB: z.number().int().min(0).max(8_000_000),
    deviceLimit: z.number().int().min(0),
    inboundIds: z.array(z.number().int().positive()),
    yooKassaPriceRubles: z.number().min(0.01, 'pages.plans.rublePriceRequired'),
    plategaPriceRubles: z.number().min(0.01, 'pages.plans.rublePriceRequired'),
    starsPrice: z.number().int().min(1),
    enabled: z.boolean(),
    sortOrder: z.number().int().min(0),
  })
  .superRefine((values, context) => {
    if (values.enabled && values.inboundIds.length === 0) {
      context.addIssue({
        code: 'custom',
        path: ['inboundIds'],
        message: 'pages.plans.inboundRequired',
      });
    }
  });

export type PlanFormValues = z.infer<typeof PlanFormSchema>;
