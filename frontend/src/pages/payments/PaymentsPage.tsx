import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Card,
  Col,
  ConfigProvider,
  Empty,
  Input,
  Layout,
  Row,
  Spin,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckCircleOutlined,
  CopyOutlined,
  CreditCardOutlined,
  ReloadOutlined,
  SaveOutlined,
  SafetyCertificateOutlined,
  StarFilled,
} from '@ant-design/icons';

import { HttpUtil } from '@/utils';
import type { Msg } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/layouts/AppSidebar';
import './PaymentsPage.css';

type ProviderID = 'platega' | 'yookassa' | 'stars';
type Provider = {
  provider: ProviderID;
  enabled: boolean;
  merchantId: string;
  hasSecret: boolean;
  secret?: string;
  clearSecret?: boolean;
};
type PaymentOrder = {
  id: number;
  uid: string;
  tgId: number;
  planName: string;
  provider: ProviderID;
  amount: number;
  currency: string;
  status: string;
  fulfillmentStatus: string;
  error?: string;
  createdAt: number;
};

const providerNames: Record<ProviderID, string> = {
  platega: 'Platega',
  yookassa: 'ЮKassa',
  stars: 'Telegram Stars',
};

function callbackURL(provider: 'platega' | 'yookassa') {
  const path = (window.X_UI_BASE_PATH || '/').replace(/\/?$/, '/');
  return `${window.location.origin}${path}store/api/webhooks/${provider}`;
}

function formatAmount(order: PaymentOrder) {
  if (order.currency !== 'RUB') return `${order.amount} ⭐`;
  return new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB' }).format(
    order.amount / 100,
  );
}

export default function PaymentsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const [providers, setProviders] = useState<Provider[]>([]);
  const [orders, setOrders] = useState<PaymentOrder[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [ordersLoading, setOrdersLoading] = useState(false);
  const [error, setError] = useState('');
  const [messageApi, contextHolder] = message.useMessage();
  const pageClass = `payments-page${isDark ? ' is-dark' : ''}${isUltra ? ' is-ultra' : ''}`;

  const loadProviders = useCallback(async () => {
    const result = await HttpUtil.get<Provider[]>('/panel/api/payments/config', undefined, {
      silent: true,
    });
    if (!result.success || !result.obj)
      throw new Error(result.msg || 'Failed to load payment settings');
    setProviders(result.obj.map((item) => ({ ...item, secret: '', clearSecret: false })));
  }, []);

  const loadOrders = useCallback(async (showLoading = true) => {
    if (showLoading) setOrdersLoading(true);
    const result = await HttpUtil.get<PaymentOrder[]>(
      '/panel/api/payments/orders?limit=100',
      undefined,
      { silent: true },
    );
    if (showLoading) setOrdersLoading(false);
    if (!result.success) throw new Error(result.msg || 'Failed to load payments');
    setOrders(result.obj || []);
  }, []);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      await Promise.all([loadProviders(), loadOrders()]);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load payment data');
    } finally {
      setLoading(false);
    }
  }, [loadOrders, loadProviders]);

  useEffect(() => {
    let active = true;
    const loadInitialData = async () => {
      try {
        await Promise.all([loadProviders(), loadOrders(false)]);
      } catch (cause) {
        if (active)
          setError(cause instanceof Error ? cause.message : 'Could not load payment data');
      } finally {
        if (active) setLoading(false);
      }
    };
    void loadInitialData();
    return () => {
      active = false;
    };
  }, [loadOrders, loadProviders]);

  function update(providerID: ProviderID, changes: Partial<Provider>) {
    setProviders((current) =>
      current.map((item) => (item.provider === providerID ? { ...item, ...changes } : item)),
    );
  }

  async function save() {
    setSaving(true);
    const result: Msg<Provider[]> = await HttpUtil.post(
      '/panel/api/payments/config',
      {
        providers: providers.map(({ provider, enabled, merchantId, secret, clearSecret }) => ({
          provider,
          enabled,
          merchantId,
          ...(secret ? { secret } : {}),
          ...(clearSecret ? { clearSecret: true } : {}),
        })),
      },
      { headers: { 'Content-Type': 'application/json' } },
    );
    setSaving(false);
    if (!result.success || !result.obj) return;
    setProviders(result.obj.map((item) => ({ ...item, secret: '', clearSecret: false })));
    messageApi.success(t('pages.payments.saved') || 'Настройки оплаты сохранены');
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      messageApi.success(t('copied') || 'Скопировано');
    } catch {
      messageApi.error('Не удалось скопировать адрес');
    }
  }

  const columns = useMemo<ColumnsType<PaymentOrder>>(
    () => [
      {
        title: t('pages.payments.createdAt'),
        dataIndex: 'createdAt',
        width: 150,
        render: (value: number) =>
          new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
            new Date(value),
          ),
      },
      {
        title: t('pages.payments.customer'),
        dataIndex: 'tgId',
        width: 125,
        render: (value: number) => <Typography.Text copyable>{value}</Typography.Text>,
      },
      { title: t('pages.payments.plan'), dataIndex: 'planName', width: 170 },
      {
        title: t('pages.payments.provider'),
        dataIndex: 'provider',
        width: 145,
        render: (value: ProviderID) => (
          <span className="provider-table-label">
            {value === 'stars' ? <StarFilled /> : <CreditCardOutlined />}
            {providerNames[value] || value}
          </span>
        ),
      },
      {
        title: t('pages.payments.amount'),
        dataIndex: 'amount',
        width: 115,
        render: (_: number, order) => formatAmount(order),
      },
      {
        title: t('pages.payments.status'),
        dataIndex: 'status',
        width: 120,
        render: (value: string) => (
          <Tag color={value === 'paid' ? 'green' : value === 'failed' ? 'red' : 'gold'}>
            {t(`pages.payments.${value}`) || value}
          </Tag>
        ),
      },
      {
        title: t('pages.payments.fulfillment'),
        dataIndex: 'fulfillmentStatus',
        width: 130,
        render: (value: string) => (
          <Tag color={value === 'completed' ? 'green' : value === 'failed' ? 'red' : 'blue'}>
            {t(`pages.payments.${value}`) || value}
          </Tag>
        ),
      },
      {
        title: t('pages.payments.error'),
        dataIndex: 'error',
        render: (value?: string) =>
          value ? (
            <Tooltip title={value}>
              <Typography.Text type="danger" ellipsis style={{ maxWidth: 220 }}>
                {value}
              </Typography.Text>
            </Tooltip>
          ) : (
            <Typography.Text type="secondary">—</Typography.Text>
          ),
      },
    ],
    [t],
  );

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {contextHolder}
      <Layout className={pageClass}>
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <div className="payments-content">
              <div className="payments-heading">
                <div>
                  <Typography.Title level={2}>{t('pages.payments.title')}</Typography.Title>
                  <Typography.Paragraph type="secondary">
                    {t('pages.payments.subtitle')}
                  </Typography.Paragraph>
                </div>
                <Button icon={<ReloadOutlined />} onClick={() => void refresh()} loading={loading}>
                  {t('refresh')}
                </Button>
              </div>
              {error && (
                <Alert type="error" showIcon title={t('somethingWentWrong')} description={error} />
              )}
              {loading ? (
                <div className="payments-loading">
                  <Spin size="large" />
                </div>
              ) : (
                <>
                  <div className="payments-section-head">
                    <div>
                      <Typography.Title level={3}>{t('pages.payments.providers')}</Typography.Title>
                      <Typography.Paragraph type="secondary">
                        {t('pages.payments.setupHelp')}
                      </Typography.Paragraph>
                    </div>
                    <SafetyCertificateOutlined />
                  </div>
                  <Row gutter={[16, 16]}>
                    {providers.map((provider) => (
                      <Col key={provider.provider} xs={24} xl={8}>
                        <Card
                          className={`provider-card${provider.enabled ? ' is-enabled' : ''}`}
                          bordered={false}
                        >
                          <div className="provider-card-heading">
                            <span className={`provider-mark mark-${provider.provider}`}>
                              {provider.provider === 'stars' ? (
                                <StarFilled />
                              ) : (
                                <CreditCardOutlined />
                              )}
                            </span>
                            <div>
                              <Typography.Title level={4}>
                                {providerNames[provider.provider]}
                              </Typography.Title>
                              <Typography.Text type="secondary">
                                {provider.provider === 'platega'
                                  ? 'Карты и СБП'
                                  : provider.provider === 'yookassa'
                                    ? 'Карты и быстрые платежи'
                                    : 'Оплата внутри Telegram'}
                              </Typography.Text>
                            </div>
                            <Switch
                              checked={provider.enabled}
                              onChange={(enabled) => update(provider.provider, { enabled })}
                              aria-label={t('pages.payments.enabled')}
                            />
                          </div>
                          {provider.provider !== 'stars' ? (
                            <>
                              <label className="provider-field">
                                <span>{t('pages.payments.merchantId')}</span>
                                <Input
                                  value={provider.merchantId}
                                  onChange={(event) =>
                                    update(provider.provider, { merchantId: event.target.value })
                                  }
                                  placeholder={
                                    provider.provider === 'yookassa' ? '123456' : 'merchant ID'
                                  }
                                />
                              </label>
                              <label className="provider-field">
                                <span>{t('pages.payments.secret')}</span>
                                <Input.Password
                                  value={provider.secret}
                                  onChange={(event) =>
                                    update(provider.provider, {
                                      secret: event.target.value,
                                      clearSecret: false,
                                    })
                                  }
                                  placeholder={
                                    provider.hasSecret
                                      ? t('pages.payments.secretPlaceholder')
                                      : '••••••••••••'
                                  }
                                  autoComplete="new-password"
                                />
                              </label>
                              {provider.hasSecret && (
                                <div className="saved-secret">
                                  <CheckCircleOutlined /> {t('pages.payments.secretSaved')}{' '}
                                  <Button
                                    type="link"
                                    danger
                                    size="small"
                                    onClick={() =>
                                      update(provider.provider, {
                                        secret: '',
                                        clearSecret: !provider.clearSecret,
                                      })
                                    }
                                  >
                                    {provider.clearSecret
                                      ? t('cancel')
                                      : t('pages.payments.clearSecret')}
                                  </Button>
                                </div>
                              )}
                              <div className="callback-field">
                                <span>{t('pages.payments.callback')}</span>
                                <div>
                                  <code>
                                    {callbackURL(provider.provider as 'platega' | 'yookassa')}
                                  </code>
                                  <Button
                                    type="text"
                                    icon={<CopyOutlined />}
                                    onClick={() =>
                                      void copy(
                                        callbackURL(provider.provider as 'platega' | 'yookassa'),
                                      )
                                    }
                                    aria-label={t('copy')}
                                  />
                                </div>
                              </div>
                              <Typography.Paragraph className="callback-hint" type="secondary">
                                {t('pages.payments.callbackHint')}
                              </Typography.Paragraph>
                            </>
                          ) : (
                            <Alert
                              className="stars-help"
                              type="info"
                              showIcon
                              message="Для Telegram Stars включите бота и настройте Mini App в разделе Telegram."
                            />
                          )}
                        </Card>
                      </Col>
                    ))}
                  </Row>
                  <div className="payments-save-row">
                    <Button
                      type="primary"
                      icon={<SaveOutlined />}
                      loading={saving}
                      onClick={() => void save()}
                    >
                      {saving ? t('pages.payments.saving') : t('pages.payments.save')}
                    </Button>
                  </div>

                  <div className="payments-section-head journal-head">
                    <div>
                      <Typography.Title level={3}>{t('pages.payments.orders')}</Typography.Title>
                      <Typography.Paragraph type="secondary">
                        {orders.length} {t('pages.payments.records') || 'заказов'}
                      </Typography.Paragraph>
                    </div>
                    <Button
                      icon={<ReloadOutlined />}
                      onClick={() =>
                        void loadOrders().catch((cause: unknown) =>
                          setError(
                            cause instanceof Error ? cause.message : 'Failed to refresh orders',
                          ),
                        )
                      }
                      loading={ordersLoading}
                    >
                      {t('pages.payments.refresh')}
                    </Button>
                  </div>
                  <Card className="payment-journal" bordered={false}>
                    {orders.length ? (
                      <Table<PaymentOrder>
                        rowKey="id"
                        columns={columns}
                        dataSource={orders}
                        loading={ordersLoading}
                        pagination={{ pageSize: 20, showSizeChanger: true }}
                        scroll={{ x: 1050 }}
                      />
                    ) : (
                      <Empty description={t('pages.payments.noOrders')} />
                    )}
                  </Card>
                </>
              )}
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
