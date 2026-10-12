import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  ArrowRightOutlined,
  CheckOutlined,
  CloseOutlined,
  CopyOutlined,
  CreditCardOutlined,
  DownOutlined,
  GlobalOutlined,
  HistoryOutlined,
  LoadingOutlined,
  MobileOutlined,
  QuestionCircleOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
  WifiOutlined,
} from '@ant-design/icons';

type Provider = { provider: 'platega' | 'yookassa' | 'stars'; enabled: boolean };
type Location = { id: number; name: string; protocol: string; port: number };
type Plan = {
  id: number;
  name: string;
  description: string;
  durationDays: number;
  trafficLimitBytes: number;
  deviceLimit: number;
  yooKassaPriceKopecks: number;
  plategaPriceKopecks: number;
  starsPrice: number;
  locations: Location[];
};
type Client = {
  id: number;
  name: string;
  enabled: boolean;
  expiryTime: number;
  totalGB: number;
  deviceLimit: number;
  subscriptionUrl: string;
};
type Order = {
  uid: string;
  planName: string;
  provider: string;
  amount: number;
  currency: string;
  status: string;
  fulfillmentStatus: string;
  createdAt: number;
};
type Catalog = { plans: Plan[]; providers: Provider[] };
type Account = {
  user: { id: number; first_name: string; username?: string };
  clients: Client[];
  orders: Order[];
};
type Checkout = {
  orderUid: string;
  provider: string;
  checkoutUrl: string;
  amount: number;
  currency: string;
  createdAt: number;
};
type OrderStatus = {
  uid: string;
  planName: string;
  provider: string;
  amount: number;
  currency: string;
  status: string;
  fulfillmentStatus: string;
  createdAt: number;
};

declare global {
  interface Window {
    X_UI_BASE_PATH?: string;
    Telegram?: {
      WebApp?: {
        initData: string;
        initDataUnsafe?: { user?: { first_name?: string } };
        colorScheme?: 'light' | 'dark';
        themeParams?: Record<string, string>;
        ready?: () => void;
        expand?: () => void;
        openLink?: (url: string, options?: { try_instant_view?: boolean }) => void;
        openInvoice?: (url: string, callback?: (status: string) => void) => void;
      };
    };
  }
}

const basePath = (window.X_UI_BASE_PATH || '/').replace(/\/?$/, '/');
const api = `${basePath}store/api`;
const money = (kopecks: number) =>
  new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    maximumFractionDigits: 2,
  }).format(kopecks / 100);
const date = (timestamp: number) =>
  timestamp
    ? new Intl.DateTimeFormat('ru-RU', { dateStyle: 'medium' }).format(new Date(timestamp))
    : '—';
const gb = (bytes: number) =>
  bytes === 0 ? 'Без ограничений' : `${Math.floor(bytes / 1024 ** 3)} ГБ`;
const iconStyle = (fontSize: number) => ({ fontSize });

function tgApp() {
  return window.Telegram?.WebApp;
}

async function request<T>(path: string, initData: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (initData) headers.set('X-Telegram-Init-Data', initData);
  if (init?.method && init.method !== 'GET') {
    headers.set('Content-Type', 'application/json');
    const csrf = document.querySelector<HTMLMetaElement>('meta[name="csrf-token"]')?.content;
    if (csrf) headers.set('X-CSRF-Token', csrf);
  }
  const response = await fetch(`${api}${path}`, { ...init, headers, credentials: 'same-origin' });
  const body = (await response.json().catch(() => ({}))) as T & { error?: string };
  if (!response.ok) throw new Error(body.error || `Ошибка запроса (${response.status})`);
  return body;
}

function providerLabel(provider: string) {
  if (provider === 'platega') return 'Platega';
  if (provider === 'yookassa') return 'ЮKassa';
  return 'Telegram Stars';
}

function priceFor(plan: Plan, provider: Provider['provider']) {
  if (provider === 'platega')
    return { amount: plan.plategaPriceKopecks, label: money(plan.plategaPriceKopecks) };
  if (provider === 'yookassa')
    return { amount: plan.yooKassaPriceKopecks, label: money(plan.yooKassaPriceKopecks) };
  return { amount: plan.starsPrice, label: `${plan.starsPrice} ⭐` };
}

export default function StorePage() {
  const webApp = useMemo(() => tgApp(), []);
  const initData = webApp?.initData || '';
  const [catalog, setCatalog] = useState<Catalog>({ plans: [], providers: [] });
  const [account, setAccount] = useState<Account | null>(null);
  const [selectedPlan, setSelectedPlan] = useState<number | null>(null);
  const [selectedClient, setSelectedClient] = useState<number | null>(null);
  const [activeOrder, setActiveOrder] = useState<OrderStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [buying, setBuying] = useState(false);
  const [error, setError] = useState('');
  const [copied, setCopied] = useState<number | null>(null);
  const [showHistory, setShowHistory] = useState(false);
  const [showAccount, setShowAccount] = useState(false);

  const refreshAccount = useCallback(async () => {
    if (!initData) return;
    const result = await request<Account>('/me', initData);
    setAccount(result);
  }, [initData]);

  const refreshOrder = useCallback(
    async (uid: string) => {
      const result = await request<{ order: OrderStatus }>(
        `/orders/${encodeURIComponent(uid)}`,
        initData,
      );
      setActiveOrder(result.order);
      if (result.order.status === 'paid' && result.order.fulfillmentStatus === 'completed') {
        await refreshAccount();
      }
    },
    [initData, refreshAccount],
  );

  useEffect(() => {
    webApp?.ready?.();
    webApp?.expand?.();
    document.documentElement.dataset.scheme = webApp?.colorScheme || 'light';
    void (async () => {
      try {
        const result = await request<Catalog>('/catalog', '');
        setCatalog(result);
        if (initData) await refreshAccount();
        const uid = new URLSearchParams(location.search).get('order');
        if (uid && initData) await refreshOrder(uid);
      } catch (cause) {
        setError(cause instanceof Error ? cause.message : 'Не удалось загрузить каталог');
      } finally {
        setLoading(false);
      }
    })();
  }, [initData, refreshAccount, refreshOrder, webApp]);

  useEffect(() => {
    const fulfillmentCanRetry =
      activeOrder?.status === 'paid' &&
      activeOrder.fulfillmentStatus !== 'completed' &&
      activeOrder.fulfillmentStatus !== 'failed';
    if (!activeOrder || !initData || (activeOrder.status !== 'pending' && !fulfillmentCanRetry))
      return;
    const timer = window.setInterval(() => void refreshOrder(activeOrder.uid), 2500);
    return () => window.clearInterval(timer);
  }, [activeOrder, initData, refreshOrder]);

  const plan = catalog.plans.find((item) => item.id === selectedPlan);
  const enabledProviders = catalog.providers.filter((provider) => provider.enabled);
  const displayProvider = enabledProviders[0]?.provider;
  const selectedClientRecord = account?.clients.find((client) => client.id === selectedClient);
  const displayName =
    webApp?.initDataUnsafe?.user?.first_name || account?.user.first_name || 'друг';

  async function startCheckout(provider: Provider['provider']) {
    if (!plan || !initData || buying) return;
    if ((account?.clients.length || 0) > 0 && selectedClient === null) {
      setError('Выберите VPN-профиль, к которому привязать покупку.');
      return;
    }
    setBuying(true);
    setError('');
    try {
      const order = await request<Checkout>('/checkout', initData, {
        method: 'POST',
        body: JSON.stringify({ planId: plan.id, clientId: selectedClient || 0, provider }),
      });
      setActiveOrder({
        ...order,
        uid: order.orderUid,
        planName: plan.name,
        status: 'pending',
        fulfillmentStatus: 'pending',
        createdAt: order.createdAt,
      });
      if (provider === 'stars' && webApp?.openInvoice) {
        webApp.openInvoice(order.checkoutUrl, (status) => {
          if (status === 'paid' || status === 'pending') void refreshOrder(order.orderUid);
        });
      } else if (webApp?.openLink) {
        webApp.openLink(order.checkoutUrl);
      } else {
        window.location.assign(order.checkoutUrl);
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Не удалось создать платёж');
    } finally {
      setBuying(false);
    }
  }

  async function copySubscription(client: Client) {
    try {
      await navigator.clipboard.writeText(client.subscriptionUrl);
      setCopied(client.id);
      window.setTimeout(() => setCopied(null), 1800);
    } catch {
      setError('Не получилось скопировать ссылку. Нажмите на неё, чтобы открыть.');
    }
  }

  return (
    <main className="store-page">
      <header className="store-header">
        <a className="store-brand" href="#top" aria-label="YTG VPN">
          <span className="store-brand-mark">
            <WifiOutlined style={iconStyle(20)} />
          </span>
          <span>
            ytg<span className="brand-accent">vpn</span>
          </span>
        </a>
        <div className="store-header-actions">
          <button
            className="icon-button"
            type="button"
            aria-label="История покупок"
            onClick={() => setShowHistory(true)}
          >
            <HistoryOutlined style={iconStyle(19)} />
          </button>
          <button className="profile-button" type="button" onClick={() => setShowAccount(true)}>
            <span className="profile-avatar">{displayName.slice(0, 1).toUpperCase()}</span>
            <span className="profile-name">{displayName}</span>
            <DownOutlined style={iconStyle(13)} />
          </button>
        </div>
      </header>

      <section className="store-hero" id="top">
        <div className="hero-copy">
          <span className="eyebrow">
            <ThunderboltOutlined style={iconStyle(14)} /> ЛИЧНЫЙ КАБИНЕТ
          </span>
          <h1>
            Быстрый и спокойный
            <br />
            <span>интернет без границ</span>
          </h1>
          <p>
            Выберите план, оплатите удобным способом — подключение появится в вашем профиле
            автоматически.
          </p>
        </div>
        <div className="hero-illustration" aria-hidden="true">
          <div className="hero-orbit orbit-one" />
          <div className="hero-orbit orbit-two" />
          <span className="hero-globe">
            <GlobalOutlined style={iconStyle(74)} />
          </span>
          <span className="hero-shield">
            <SafetyCertificateOutlined style={iconStyle(27)} />
          </span>
          <span className="hero-spark spark-a">✳</span>
          <span className="hero-spark spark-b">✦</span>
        </div>
      </section>

      {!initData && (
        <div className="store-alert">
          <QuestionCircleOutlined style={iconStyle(18)} /> Откройте эту страницу через
          Telegram-бота, чтобы увидеть свой профиль и оформить подписку.
        </div>
      )}
      {error && (
        <div className="store-alert is-error">
          <CloseOutlined style={iconStyle(18)} />
          <span>{error}</span>
          <button type="button" onClick={() => setError('')} aria-label="Закрыть">
            <CloseOutlined style={iconStyle(16)} />
          </button>
        </div>
      )}

      <section className="store-section-heading">
        <div>
          <span className="eyebrow">ПОДБЕРИТЕ ДОСТУП</span>
          <h2>Тарифы для вас</h2>
        </div>
        <span className="secure-note">
          <SafetyCertificateOutlined style={iconStyle(16)} /> Защищённая оплата
        </span>
      </section>

      {loading ? (
        <div className="store-loading">
          <LoadingOutlined className="spin" style={iconStyle(24)} /> Загружаем планы…
        </div>
      ) : catalog.plans.length === 0 ? (
        <div className="store-empty">
          <span className="empty-icon">
            <WifiOutlined style={iconStyle(22)} />
          </span>
          <h3>Тарифы пока не опубликованы</h3>
          <p>Загляните сюда немного позже.</p>
        </div>
      ) : (
        <section className="plan-grid" aria-label="Тарифы">
          {catalog.plans.map((item) => (
            <button
              type="button"
              key={item.id}
              className={`store-plan-card${selectedPlan === item.id ? ' is-selected' : ''}`}
              onClick={() => {
                setSelectedPlan(item.id);
                setError('');
              }}
            >
              <span className="plan-card-top">
                <span className="plan-card-icon">
                  <WifiOutlined style={iconStyle(19)} />
                </span>
                {selectedPlan === item.id && (
                  <span className="selected-check">
                    <CheckOutlined style={iconStyle(14)} />
                  </span>
                )}
              </span>
              <span className="plan-title">{item.name}</span>
              <span className="plan-description">
                {item.description || 'Надёжное подключение для повседневных задач.'}
              </span>
              <span className="plan-price-row">
                <strong>{displayProvider ? priceFor(item, displayProvider).label : '—'}</strong>
                <span>/ {item.durationDays} дней</span>
              </span>
              <span className="plan-divider" />
              <span className="plan-feature">
                <CheckOutlined style={iconStyle(15)} /> {gb(item.trafficLimitBytes)} трафика
              </span>
              <span className="plan-feature">
                <MobileOutlined style={iconStyle(15)} /> {item.deviceLimit || 'Без лимита'}{' '}
                устройств
              </span>
              <span className="plan-feature">
                <GlobalOutlined style={iconStyle(15)} /> {item.locations.length}{' '}
                {item.locations.length === 1 ? 'локация' : 'локации'}
              </span>
              <span className="plan-locations">
                {item.locations.map((location) => (
                  <span key={location.id}>{location.name}</span>
                ))}
              </span>
            </button>
          ))}
        </section>
      )}

      {plan && (
        <section className="purchase-panel" aria-live="polite">
          <div className="purchase-summary">
            <span className="eyebrow">ВАШ ВЫБОР</span>
            <h3>{plan.name}</h3>
            <p>
              {plan.durationDays} дней · {gb(plan.trafficLimitBytes)} · {plan.locations.length}{' '}
              локации
            </p>
          </div>
          <div className="purchase-controls">
            {account && account.clients.length > 0 && (
              <label className="client-select-label">
                Куда привязать подписку
                <select
                  value={selectedClient ?? ''}
                  onChange={(event) =>
                    setSelectedClient(event.target.value ? Number(event.target.value) : null)
                  }
                >
                  <option value="">Выберите VPN-профиль</option>
                  {account.clients.map((client) => (
                    <option key={client.id} value={client.id}>
                      {client.name}
                      {client.enabled ? '' : ' · отключён'}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {account && account.clients.length === 0 && (
              <span className="muted-note new-profile-note">
                У вас пока нет VPN-профиля. Создадим его автоматически после оплаты.
              </span>
            )}
            <div className="provider-buttons">
              {enabledProviders.length === 0 ? (
                <span className="muted-note">Способы оплаты временно недоступны.</span>
              ) : (
                enabledProviders.map((provider) => {
                  const price = priceFor(plan, provider.provider);
                  return (
                    <button
                      key={provider.provider}
                      type="button"
                      className={`pay-button pay-${provider.provider}`}
                      disabled={
                        buying ||
                        !initData ||
                        (!!account?.clients.length && selectedClient === null)
                      }
                      onClick={() => void startCheckout(provider.provider)}
                    >
                      {buying ? (
                        <LoadingOutlined className="spin" style={iconStyle(17)} />
                      ) : provider.provider === 'stars' ? (
                        <span className="stars-icon">⭐</span>
                      ) : (
                        <CreditCardOutlined style={iconStyle(17)} />
                      )}
                      <span>
                        <strong>{providerLabel(provider.provider)}</strong>
                        <small>{price.label}</small>
                      </span>
                      <ArrowRightOutlined style={iconStyle(17)} />
                    </button>
                  );
                })
              )}
            </div>
            {selectedClientRecord && (
              <span className="muted-note">
                Покупка продлит профиль «{selectedClientRecord.name}» и применит выбранные локации
                тарифа.
              </span>
            )}
          </div>
        </section>
      )}

      {activeOrder && (
        <section className={`order-notice${activeOrder.status === 'paid' ? ' is-paid' : ''}`}>
          <span className="order-status-icon">
            {activeOrder.status === 'paid' && activeOrder.fulfillmentStatus === 'completed' ? (
              <CheckOutlined style={iconStyle(18)} />
            ) : (
              <LoadingOutlined className="spin" style={iconStyle(18)} />
            )}
          </span>
          <span>
            <strong>
              {activeOrder.status === 'paid'
                ? activeOrder.fulfillmentStatus === 'completed'
                  ? 'Доступ готов'
                  : activeOrder.fulfillmentStatus === 'failed'
                    ? 'Оплата принята, выдача требует внимания'
                    : 'Оплата получена, подключаем…'
                : 'Ожидаем подтверждение оплаты'}
            </strong>
            <small>
              {activeOrder.planName} ·{' '}
              {activeOrder.currency === 'RUB'
                ? money(activeOrder.amount)
                : `${activeOrder.amount} ⭐`}
            </small>
          </span>
          <button type="button" className="text-button" onClick={() => setShowAccount(true)}>
            Мой доступ
          </button>
        </section>
      )}

      <section className="trust-row">
        <div>
          <SafetyCertificateOutlined style={iconStyle(18)} />
          <span>
            <strong>Привязка к Telegram</strong>
            <small>Покупки связаны с вашим аккаунтом</small>
          </span>
        </div>
        <div>
          <CreditCardOutlined style={iconStyle(18)} />
          <span>
            <strong>Безопасная оплата</strong>
            <small>Проверка платежа у провайдера</small>
          </span>
        </div>
        <div>
          <MobileOutlined style={iconStyle(18)} />
          <span>
            <strong>Поддержка устройств</strong>
            <small>Подключите свои приложения</small>
          </span>
        </div>
      </section>

      <footer className="store-footer">
        <span>ytgvpn · защищённый доступ</span>
        <button type="button" className="text-button" onClick={() => setShowHistory(true)}>
          История платежей <ArrowRightOutlined style={iconStyle(14)} />
        </button>
      </footer>

      {(showHistory || showAccount) && (
        <div className="sheet-backdrop">
          <button
            className="sheet-overlay"
            type="button"
            aria-label="Закрыть окно"
            onClick={() => {
              setShowHistory(false);
              setShowAccount(false);
            }}
          />
          <section
            className="account-sheet"
            role="dialog"
            aria-modal="true"
            aria-label={showHistory ? 'История платежей' : 'Мой профиль'}
          >
            <div className="sheet-heading">
              <div>
                <span className="eyebrow">{showHistory ? 'ПОКУПКИ' : 'ПРОФИЛЬ'}</span>
                <h2>{showHistory ? 'История платежей' : 'Мой доступ'}</h2>
              </div>
              <button
                className="icon-button"
                type="button"
                onClick={() => {
                  setShowHistory(false);
                  setShowAccount(false);
                }}
              >
                <CloseOutlined style={iconStyle(19)} />
              </button>
            </div>
            {showHistory ? (
              <div className="history-list">
                {!account?.orders.length ? (
                  <div className="sheet-empty">Платежей пока нет.</div>
                ) : (
                  account.orders.map((order) => (
                    <article className="history-item" key={order.uid}>
                      <span className="history-dot" />
                      <span className="history-info">
                        <strong>{order.planName}</strong>
                        <small>
                          {date(order.createdAt)} · {providerLabel(order.provider)}
                        </small>
                      </span>
                      <span className="history-amount">
                        {order.currency === 'RUB' ? money(order.amount) : `${order.amount} ⭐`}
                        <small>
                          {order.status === 'failed'
                            ? 'Ошибка оплаты'
                            : order.status === 'paid'
                              ? order.fulfillmentStatus === 'completed'
                                ? 'Доступ выдан'
                                : order.fulfillmentStatus === 'failed'
                                  ? 'Нужна помощь оператора'
                                  : 'Оплачен, доступ готовится'
                              : 'Ожидает оплаты'}
                        </small>
                      </span>
                    </article>
                  ))
                )}
              </div>
            ) : (
              <div className="client-list">
                {!account?.clients.length ? (
                  <div className="sheet-empty">После первой покупки профиль появится здесь.</div>
                ) : (
                  account.clients.map((client) => (
                    <article className="client-card" key={client.id}>
                      <span className="client-card-icon">
                        <WifiOutlined style={iconStyle(18)} />
                      </span>
                      <span className="client-card-info">
                        <strong>{client.name}</strong>
                        <small>
                          {client.enabled ? 'Активен' : 'Отключён'} ·{' '}
                          {client.expiryTime < 0
                            ? 'после первого подключения'
                            : client.expiryTime
                              ? `до ${date(client.expiryTime)}`
                              : 'без срока'}{' '}
                          · {client.totalGB || 'без лимита'}
                          {client.totalGB ? ' ГБ' : ''}
                        </small>
                      </span>
                      <button
                        type="button"
                        className="icon-button copy-button"
                        title="Скопировать ссылку подписки"
                        disabled={!client.subscriptionUrl}
                        onClick={() => void copySubscription(client)}
                      >
                        {copied === client.id ? (
                          <CheckOutlined style={iconStyle(17)} />
                        ) : (
                          <CopyOutlined style={iconStyle(17)} />
                        )}
                      </button>
                      {client.subscriptionUrl ? (
                        <a
                          className="client-open-link"
                          href={client.subscriptionUrl}
                          target="_blank"
                          rel="noreferrer"
                        >
                          Открыть подписку
                        </a>
                      ) : (
                        <span className="client-open-link is-unavailable">
                          Ссылка появится после настройки домена подписки
                        </span>
                      )}
                    </article>
                  ))
                )}
              </div>
            )}
            <div className="sheet-hint">
              <SafetyCertificateOutlined style={iconStyle(16)} /> Доступ и покупки привязаны к
              вашему Telegram-аккаунту.
            </div>
          </section>
        </div>
      )}
    </main>
  );
}
