import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { FormProvider } from 'react-hook-form';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Form,
  Input,
  InputNumber,
  Layout,
  Modal,
  Result,
  Row,
  Select,
  Spin,
  Statistic,
  Switch,
  Tag,
  Typography,
  message,
} from 'antd';
import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  DeleteOutlined,
  EditOutlined,
  GlobalOutlined,
  PlusOutlined,
  ThunderboltOutlined,
  UserOutlined,
} from '@ant-design/icons';

import { FormField, useZodForm } from '@/components/form/rhf';
import { useInboundOptions } from '@/api/queries/useInboundOptions';
import { usePlanMutations } from '@/api/queries/usePlanMutations';
import { usePlansQuery } from '@/api/queries/usePlansQuery';
import { useTheme } from '@/hooks/useTheme';
import type { InboundOption } from '@/schemas/client';
import { PlanFormSchema, type Plan, type PlanFormValues } from '@/schemas/plan';
import AppSidebar from '@/layouts/AppSidebar';
import './PlansPage.css';

const GIB = 1024 ** 3;
const EMPTY_INBOUNDS: InboundOption[] = [];

const EMPTY_FORM: PlanFormValues = {
  name: '',
  description: '',
  durationDays: 30,
  trafficLimitGB: 0,
  deviceLimit: 0,
  inboundIds: [],
  yooKassaPriceRubles: 0,
  plategaPriceRubles: 0,
  starsPrice: 0,
  enabled: true,
  sortOrder: 0,
};

function formValues(plan?: Plan): PlanFormValues {
  if (!plan) return EMPTY_FORM;
  return {
    name: plan.name,
    description: plan.description,
    durationDays: plan.durationDays,
    trafficLimitGB: Math.round(plan.trafficLimitBytes / GIB),
    deviceLimit: plan.deviceLimit,
    inboundIds: plan.inboundIds,
    yooKassaPriceRubles: plan.yooKassaPriceKopecks / 100,
    plategaPriceRubles: plan.plategaPriceKopecks / 100,
    starsPrice: plan.starsPrice,
    enabled: plan.enabled,
    sortOrder: plan.sortOrder,
  };
}

function formatRubles(amountKopecks: number, locale: string) {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'RUB',
    maximumFractionDigits: 2,
  }).format(amountKopecks / 100);
}

export default function PlansPage() {
  const { t, i18n } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { data: plans = [], isFetching, isError, error, refetch } = usePlansQuery();
  const inboundsQuery = useInboundOptions();
  const inbounds = inboundsQuery.data ?? EMPTY_INBOUNDS;
  const mutations = usePlanMutations();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Plan | undefined>();
  const form = useZodForm(PlanFormSchema, { defaultValues: EMPTY_FORM });
  const planEnabled = form.watch('enabled');
  const inboundById = useMemo(() => new Map(inbounds.map((item) => [item.id, item])), [inbounds]);

  function openForm(plan?: Plan) {
    setEditing(plan);
    form.reset(formValues(plan));
    setFormOpen(true);
  }

  async function save(values: PlanFormValues) {
    const result = editing
      ? await mutations.update(editing.id, values)
      : await mutations.create(values);
    if (!result.success) return;
    setFormOpen(false);
    messageApi.success(t('pages.plans.saved'));
  }

  function confirmDelete(plan: Plan) {
    modal.confirm({
      title: t('delete'),
      content: t('pages.plans.deleteConfirm', { name: plan.name }),
      okText: t('delete'),
      okButtonProps: { danger: true },
      cancelText: t('cancel'),
      onOk: async () => {
        const result = await mutations.remove(plan.id);
        if (!result.success) return Promise.reject(new Error(result.msg));
        messageApi.success(t('pages.plans.deleted'));
      },
    });
  }

  const pageClass = `plans-page${isDark ? ' is-dark' : ''}${isUltra ? ' is-ultra' : ''}`;
  const ready = plans.length > 0 || isError || !isFetching;
  const inboundOptions = inbounds.map((inbound) => ({
    value: inbound.id,
    label: `${inbound.remark || inbound.tag} · ${inbound.protocol?.toUpperCase() || '—'} :${inbound.port}`,
  }));

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <Spin spinning={!ready}>
              {isError ? (
                <Result
                  status="error"
                  title={t('somethingWentWrong')}
                  subTitle={error instanceof Error ? error.message : undefined}
                  extra={<Button onClick={() => void refetch()}>{t('refresh')}</Button>}
                />
              ) : (
                <div className="plans-content">
                  <div className="plans-heading">
                    <div>
                      <Typography.Title level={2}>{t('pages.plans.title')}</Typography.Title>
                      <Typography.Paragraph type="secondary">
                        {t('pages.plans.intro')}
                      </Typography.Paragraph>
                    </div>
                    <Button
                      type="primary"
                      icon={<PlusOutlined />}
                      disabled={inbounds.length === 0}
                      onClick={() => openForm()}
                    >
                      {t('pages.plans.createPlan')}
                    </Button>
                  </div>

                  {inboundsQuery.isError && (
                    <Alert
                      type="error"
                      showIcon
                      title={t('somethingWentWrong')}
                      description={inboundsQuery.error.message}
                    />
                  )}
                  {!inboundsQuery.isError && inbounds.length === 0 && (
                    <Alert
                      type="info"
                      showIcon
                      title={t('pages.plans.noInbounds')}
                      description={t('pages.plans.noInboundsDescription')}
                    />
                  )}

                  {plans.length === 0 ? (
                    <Card className="plans-empty-card">
                      <Empty
                        image={Empty.PRESENTED_IMAGE_SIMPLE}
                        description={
                          <div>
                            <Typography.Title level={4} className="plans-empty-title">
                              {t('pages.plans.emptyTitle')}
                            </Typography.Title>
                            <Typography.Paragraph type="secondary">
                              {t('pages.plans.emptyDescription')}
                            </Typography.Paragraph>
                          </div>
                        }
                      >
                        <Button
                          type="primary"
                          disabled={inbounds.length === 0}
                          icon={<PlusOutlined />}
                          onClick={() => openForm()}
                        >
                          {t('pages.plans.createPlan')}
                        </Button>
                      </Empty>
                    </Card>
                  ) : (
                    <Row gutter={[18, 18]}>
                      {plans.map((plan) => (
                        <Col key={plan.id} xs={24} md={12} xl={8}>
                          <Card className="plan-card" bordered={false}>
                            <div className="plan-card-topline">
                              <span className="plan-card-mark">
                                <ThunderboltOutlined />
                              </span>
                              <Tag color={plan.enabled ? 'green' : 'default'}>
                                {t(plan.enabled ? 'enabled' : 'disabled')}
                              </Tag>
                            </div>
                            <Typography.Title level={3} className="plan-card-name">
                              {plan.name}
                            </Typography.Title>
                            {plan.description ? (
                              <Typography.Paragraph
                                type="secondary"
                                className="plan-card-description"
                                ellipsis={{ rows: 2 }}
                              >
                                {plan.description}
                              </Typography.Paragraph>
                            ) : (
                              <Typography.Paragraph
                                type="secondary"
                                className="plan-card-description plan-card-description-empty"
                              >
                                {t('pages.plans.noDescription')}
                              </Typography.Paragraph>
                            )}
                            <div className="plan-card-stats">
                              <Statistic
                                title={t('pages.plans.durationDays')}
                                value={plan.durationDays}
                                prefix={<ClockCircleOutlined />}
                                suffix={t('pages.plans.daysUnit')}
                              />
                              <Statistic
                                title={t('pages.plans.trafficLimitGB')}
                                value={
                                  plan.trafficLimitBytes === 0
                                    ? t('unlimited')
                                    : Math.round(plan.trafficLimitBytes / GIB)
                                }
                                suffix={plan.trafficLimitBytes === 0 ? undefined : 'GB'}
                              />
                              <Statistic
                                title={t('pages.plans.deviceLimit')}
                                value={plan.deviceLimit === 0 ? t('unlimited') : plan.deviceLimit}
                                prefix={<UserOutlined />}
                              />
                            </div>
                            <div className="plan-card-locations">
                              <Typography.Text className="plan-section-label">
                                <GlobalOutlined /> {t('pages.plans.locations')}
                              </Typography.Text>
                              <div className="plan-location-tags">
                                {plan.inboundIds.map((id) => {
                                  const inbound = inboundById.get(id);
                                  return (
                                    <Tag key={id}>
                                      {inbound?.remark || inbound?.tag || `#${id}`}
                                    </Tag>
                                  );
                                })}
                                {plan.inboundIds.length === 0 && (
                                  <Typography.Text type="secondary">
                                    {t('pages.plans.noIncludedInbounds')}
                                  </Typography.Text>
                                )}
                              </div>
                            </div>
                            <div className="plan-card-prices">
                              <div className="plan-card-provider-price">
                                <Typography.Text type="secondary">
                                  {t('pages.plans.yooKassaPrice')}
                                </Typography.Text>
                                <Typography.Title level={3} className="plan-card-price">
                                  {formatRubles(plan.yooKassaPriceKopecks, i18n.language)}
                                </Typography.Title>
                              </div>
                              <div className="plan-card-provider-price">
                                <Typography.Text type="secondary">
                                  {t('pages.plans.plategaPrice')}
                                </Typography.Text>
                                <Typography.Title level={3} className="plan-card-price">
                                  {formatRubles(plan.plategaPriceKopecks, i18n.language)}
                                </Typography.Title>
                              </div>
                              {plan.starsPrice > 0 && (
                                <div className="plan-stars-price">
                                  <Typography.Text type="secondary">
                                    {t('pages.plans.starsPrice')}
                                  </Typography.Text>
                                  <Typography.Title level={4} className="plan-card-price">
                                    {plan.starsPrice} ⭐
                                  </Typography.Title>
                                </div>
                              )}
                            </div>
                            <div className="plan-card-actions">
                              <Button icon={<EditOutlined />} onClick={() => openForm(plan)}>
                                {t('edit')}
                              </Button>
                              <Button
                                danger
                                icon={<DeleteOutlined />}
                                onClick={() => confirmDelete(plan)}
                              >
                                {t('delete')}
                              </Button>
                            </div>
                          </Card>
                        </Col>
                      ))}
                    </Row>
                  )}
                </div>
              )}
            </Spin>
          </Layout.Content>
        </Layout>
      </Layout>

      <Modal
        open={formOpen}
        title={editing ? t('edit') : t('pages.plans.createPlan')}
        okText={editing ? t('save') : t('create')}
        cancelText={t('cancel')}
        confirmLoading={mutations.saving}
        onCancel={() => setFormOpen(false)}
        onOk={() => void form.handleSubmit(save)()}
        destroyOnHidden
      >
        <FormProvider {...form}>
          <Form layout="vertical" onFinish={form.handleSubmit(save)}>
            <FormField name="name" label={t('pages.plans.planName')} required>
              <Input maxLength={120} />
            </FormField>
            <FormField name="description" label={t('pages.plans.planDescription')}>
              <Input.TextArea rows={2} maxLength={2000} />
            </FormField>
            <Row gutter={12}>
              <Col span={12}>
                <FormField
                  name="durationDays"
                  label={t('pages.plans.durationDays')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                  required
                >
                  <InputNumber min={1} max={3650} precision={0} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={12}>
                <FormField
                  name="trafficLimitGB"
                  label={t('pages.plans.trafficLimitGB')}
                  extra={t('pages.plans.trafficLimitHelp')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                >
                  <InputNumber min={0} precision={0} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={12}>
                <FormField
                  name="deviceLimit"
                  label={t('pages.plans.deviceLimit')}
                  extra={t('pages.plans.deviceLimitHelp')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                >
                  <InputNumber min={0} precision={0} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={12}>
                <FormField
                  name="yooKassaPriceRubles"
                  label={t('pages.plans.yooKassaPrice')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                  required
                >
                  <InputNumber min={0.01} step={10} precision={2} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={12}>
                <FormField
                  name="plategaPriceRubles"
                  label={t('pages.plans.plategaPrice')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                  required
                >
                  <InputNumber min={0.01} step={10} precision={2} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={12}>
                <FormField
                  name="starsPrice"
                  label={t('pages.plans.starsPrice')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                  required
                >
                  <InputNumber min={1} precision={0} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={12}>
                <FormField
                  name="sortOrder"
                  label={t('pages.plans.sortOrder')}
                  transform={{ output: (value) => Number(value ?? 0) }}
                >
                  <InputNumber min={0} precision={0} style={{ width: '100%' }} />
                </FormField>
              </Col>
              <Col span={24}>
                <FormField
                  name="inboundIds"
                  label={t('pages.plans.locations')}
                  required={planEnabled}
                >
                  <Select
                    mode="multiple"
                    options={inboundOptions}
                    placeholder={t('pages.plans.selectInbounds')}
                    showSearch={{
                      filterOption: (input, option) =>
                        String(option?.label ?? '')
                          .toLowerCase()
                          .includes(input.toLowerCase()),
                    }}
                  />
                </FormField>
              </Col>
              <Col span={24}>
                <FormField name="enabled" label={t('enabled')} valueProp="checked">
                  <Switch />
                </FormField>
              </Col>
            </Row>
            {inbounds.length > 0 && (
              <Typography.Paragraph type="secondary" className="plan-form-hint">
                <CheckCircleOutlined /> {t('pages.plans.fixedLocationsHint')}
              </Typography.Paragraph>
            )}
          </Form>
        </FormProvider>
      </Modal>
    </ConfigProvider>
  );
}
