import { useState } from "react";
import { Link } from "react-router-dom";
import {
  ArrowDownToLine,
  ArrowRight,
  ChartNoAxesCombined,
  Coins,
  Eye,
  ShoppingBag,
  Wallet,
  Package,
  Clock3,
  Mail,
  CalendarDays,
  Moon,
} from "lucide-react";
import {
  ResponsiveContainer,
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  BarChart,
  Bar,
} from "recharts";
import {
  useAdminQuery,
  orderNet,
  yuan,
  number,
  exportCSV,
  type DashboardData,
  type AdminOrder,
} from "./api";
import {
  Banner,
  Busy,
  Empty,
  PageHeading,
  QueryError,
  Status,
  shortDate,
} from "./ui";
export function RecentOrders({ orders }: { orders: AdminOrder[] }) {
  if (!orders.length)
    return (
      <Empty
        title="等待第一笔好生意"
        text="成交后，订单、收入与利润会自动汇总到这里。"
      />
    );
  return (
    <div className="a-table-wrap">
      <table>
        <thead>
          <tr>
            <th>订单 / 时间</th>
            <th>订单金额</th>
            <th>经营净利润</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {orders.map((o) => (
            <tr key={o.id}>
              <td>
                <strong>{o.order_no}</strong>
                <small>{shortDate(o.created_at)}</small>
              </td>
              <td>{yuan(o.total_cents)}</td>
              <td className="a-profit">
                {orderNet(o) == null
                  ? o.paid_at
                    ? "待补全"
                    : "—"
                  : yuan(orderNet(o)!)}
              </td>
              <td>
                <Status value={o.status} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
export default function Dashboard({ base }: { base: string }) {
  const [days, setDays] = useState(7);
  const [chart, setChart] = useState<"sales" | "traffic">("sales");
  const q = useAdminQuery<DashboardData>(`/dashboard?days=${days}`);
  if (q.loading && !q.data) return <Busy />;
  if (q.error) return <QueryError error={q.error} reload={q.reload} />;
  const d = q.data!;
  const metrics = [
    {
      label: "成交收入",
      value: yuan(d.revenue_cents),
      icon: Wallet,
      meta: `今日 ${yuan(d.today.revenue_cents)}`,
      color: "blue",
    },
    {
      label: "经营净利润",
      value: yuan(d.net_profit_cents),
      icon: Coins,
      meta: `手续费 ${yuan(d.fee_cents)} · 佣金 ${yuan(d.commission_cents)}`,
      color: "green",
    },
    {
      label: "成交订单",
      value: number(d.paid_orders),
      icon: ShoppingBag,
      meta: `今日 ${d.today.paid_orders} 笔成交`,
      color: "violet",
    },
    {
      label: "日访客累计 · UV",
      value: number(d.uv),
      icon: Eye,
      meta: `浏览量 ${number(d.pv)} 次 · PV`,
      color: "amber",
    },
  ];
  return (
    <>
      <PageHeading
        eyebrow="A LITTLE GROWTH, EVERY DAY"
        title="经营概览"
        description="把每一份好生意，都记得清清楚楚。"
      >
        <span className="a-period">
          <CalendarDays size={15} />
          <select
            aria-label="统计周期"
            value={days}
            onChange={(e) => setDays(Number(e.target.value))}
          >
            <option value={7}>最近 7 天</option>
            <option value={30}>最近 30 天</option>
            <option value={90}>最近 90 天</option>
          </select>
        </span>
        <button
          className="a-button"
          onClick={() =>
            exportCSV("Wybo-经营报表.csv", [
              [
                "日期",
                "收入（分）",
                "成本（分）",
                "利润（分）",
                "成交数",
                "PV",
                "UV",
              ],
              ...d.daily.map((x) => [
                x.date,
                x.revenue_cents,
                x.cost_cents,
                x.net_profit_cents,
                x.paid_orders,
                x.pv,
                x.uv,
              ]),
            ])
          }
        >
          <ArrowDownToLine size={15} />
          导出报表
        </button>
      </PageHeading>
      {d.untracked_orders > 0 && (
        <Banner>
          有 {d.untracked_orders} 笔成交订单缺少历史成本，暂未计入利润。
        </Banner>
      )}
      {(d.unknown_net_orders > 0 || d.estimated_orders > 0) && (
        <Banner>
          {d.unknown_net_orders} 笔订单成本或手续费待补全，未计入净利润；
          {d.estimated_orders} 笔费用使用快照估算。
          <Link to={base + "/reports"}>查看财务月报</Link>
        </Banner>
      )}
      <div className="a-metrics">
        {metrics.map((m) => (
          <article className="a-metric" key={m.label}>
            <div>
              <span>{m.label}</span>
              <i className={`a-icon-${m.color}`}>
                <m.icon size={18} />
              </i>
            </div>
            <strong
              className={
                m.label === "经营净利润" && d.net_profit_cents < 0
                  ? "a-negative"
                  : ""
              }
            >
              {m.value}
            </strong>
            <small>{m.meta}</small>
          </article>
        ))}
      </div>
      <div className="a-analytics-grid">
        <section className="a-panel a-chart-panel">
          <div className="a-panel-heading">
            <div>
              <h2>
                <ChartNoAxesCombined size={18} />
                经营趋势
              </h2>
              <p>
                {chart === "sales"
                  ? "实收与经营净利润，按成交时间统计"
                  : "真实页面访问，按北京时间每日汇总"}
              </p>
            </div>
            <div className="a-segment">
              <button
                className={chart === "sales" ? "active" : ""}
                onClick={() => setChart("sales")}
              >
                收入
              </button>
              <button
                className={chart === "traffic" ? "active" : ""}
                onClick={() => setChart("traffic")}
              >
                流量
              </button>
            </div>
          </div>
          <div className="a-chart-legend">
            {chart === "sales" ? (
              <>
                <span>
                  <i style={{ background: "#6688e3" }} />
                  成交收入
                </span>
                <span>
                  <i style={{ background: "#49a38d" }} />
                  经营净利润
                </span>
              </>
            ) : (
              <>
                <span>
                  <i style={{ background: "#6688e3" }} />
                  浏览量 PV
                </span>
                <span>
                  <i style={{ background: "#a6b8ee" }} />
                  访问人数 UV
                </span>
              </>
            )}
          </div>
          <div className="a-chart">
            <ResponsiveContainer width="100%" height="100%">
              {chart === "sales" ? (
                <AreaChart
                  data={d.daily}
                  margin={{ top: 14, right: 8, left: -15, bottom: 0 }}
                >
                  <defs>
                    <linearGradient id="incomeFill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="#7896e7" stopOpacity={0.2} />
                      <stop offset="100%" stopColor="#7896e7" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid
                    vertical={false}
                    stroke="#edf0f5"
                    strokeDasharray="4 4"
                  />
                  <XAxis
                    dataKey="date"
                    tickFormatter={(v) => String(v).slice(5).replace("-", "/")}
                    axisLine={false}
                    tickLine={false}
                    tick={{ fill: "#8891a2", fontSize: 11 }}
                    minTickGap={24}
                  />
                  <YAxis
                    domain={["auto", (max: number) => Math.max(max, 10000)]}
                    tickFormatter={(v) => `¥${v / 100}`}
                    axisLine={false}
                    tickLine={false}
                    tick={{ fill: "#8891a2", fontSize: 11 }}
                  />
                  <Tooltip
                    formatter={(v, name) => [
                      yuan(Number(v)),
                      name === "revenue_cents" ? "成交收入" : "经营净利润",
                    ]}
                    labelFormatter={(v) => String(v)}
                  />
                  <Area
                    type="monotone"
                    dataKey="revenue_cents"
                    stroke="#6688e3"
                    strokeWidth={2.5}
                    fill="url(#incomeFill)"
                    isAnimationActive={false}
                  />
                  <Area
                    type="monotone"
                    dataKey="net_profit_cents"
                    stroke="#49a38d"
                    strokeWidth={2}
                    fill="transparent"
                    isAnimationActive={false}
                  />
                </AreaChart>
              ) : (
                <BarChart
                  data={d.daily}
                  margin={{ top: 14, right: 8, left: -24, bottom: 0 }}
                >
                  <CartesianGrid
                    vertical={false}
                    stroke="#edf0f5"
                    strokeDasharray="4 4"
                  />
                  <XAxis
                    dataKey="date"
                    tickFormatter={(v) => String(v).slice(5).replace("-", "/")}
                    axisLine={false}
                    tickLine={false}
                    tick={{ fill: "#8891a2", fontSize: 11 }}
                    minTickGap={24}
                  />
                  <YAxis
                    allowDecimals={false}
                    axisLine={false}
                    tickLine={false}
                    tick={{ fill: "#8891a2", fontSize: 11 }}
                  />
                  <Tooltip
                    formatter={(v, name) => [
                      Number(v),
                      name === "pv" ? "浏览量 PV" : "访问人数 UV",
                    ]}
                  />
                  <Bar
                    dataKey="pv"
                    fill="#6688e3"
                    radius={[4, 4, 0, 0]}
                    maxBarSize={20}
                    isAnimationActive={false}
                  />
                  <Bar
                    dataKey="uv"
                    fill="#a6b8ee"
                    radius={[4, 4, 0, 0]}
                    maxBarSize={20}
                    isAnimationActive={false}
                  />
                </BarChart>
              )}
            </ResponsiveContainer>
          </div>
          <div className="a-chart-footnote">
            {chart === "sales"
              ? "经营净利润 = 实收 − 退款 − 商品成本 − 手续费 − 佣金"
              : "UV 按每日访客去重，跨日累加；不统计后台请求"}
            <span>北京时间 · UTC+8</span>
          </div>
        </section>
        <section className="a-panel a-attention">
          <div className="a-panel-heading">
            <div>
              <h2>小铺待办</h2>
              <p>经营中的小事，也值得被照顾</p>
            </div>
            <Moon size={23} className="a-tiny-moon" />
          </div>
          <Link to={base + "/stock"}>
            <i className="a-icon-amber">
              <Package size={18} />
            </i>
            <div>
              <strong>库存提醒</strong>
              <span>库存不超过 {d.stock_threshold ?? 5} 份的商品</span>
            </div>
            <b>{d.low_stock_products}</b>
            <ArrowRight size={14} />
          </Link>
          <Link to={base + "/orders?status=pending"}>
            <i className="a-icon-blue">
              <Clock3 size={18} />
            </i>
            <div>
              <strong>待付款订单</strong>
              <span>等待顾客完成支付</span>
            </div>
            <b>{d.pending_orders}</b>
            <ArrowRight size={14} />
          </Link>
          <Link to={base + "/notifications"}>
            <i className="a-icon-violet">
              <Mail size={18} />
            </i>
            <div>
              <strong>推送待处理</strong>
              <span>重试后仍未发送成功</span>
            </div>
            <b>{d.failed_notifications}</b>
            <ArrowRight size={14} />
          </Link>
          <div className="a-profit-note">
            <Coins size={18} />
            <p>
              示例：售价 ¥115 − 成本 ¥105
              <br />
              <strong>零手续费、零佣金时赚 ¥10</strong>
            </p>
            <Link to={base + "/products"}>
              设置成本 <ArrowRight size={13} />
            </Link>
          </div>
        </section>
      </div>
      <div className="a-bottom-grid">
        <section className="a-panel">
          <div className="a-panel-heading">
            <div>
              <h2>最近订单</h2>
              <p>小铺最新的 5 笔订单</p>
            </div>
            <Link className="a-text-link" to={base + "/orders"}>
              全部订单 <ArrowRight size={14} />
            </Link>
          </div>
          <RecentOrders orders={d.recent_orders} />
        </section>
        <section className="a-panel">
          <div className="a-panel-heading">
            <div>
              <h2>商品表现</h2>
              <p>按统计周期内的成交金额排序</p>
            </div>
          </div>
          {d.top_products.length ? (
            <div className="a-ranking">
              {[...d.top_products]
                .sort((a, b) => b.revenue_cents - a.revenue_cents)
                .slice(0, 5)
                .map((p, i) => (
                  <div key={p.id}>
                    <b>{String(i + 1).padStart(2, "0")}</b>
                    <span>
                      <strong>{p.name}</strong>
                      <small>已售 {p.quantity} 份</small>
                    </span>
                    <strong>{yuan(p.revenue_cents)}</strong>
                  </div>
                ))}
            </div>
          ) : (
            <Empty
              title="好商品，等着被发现"
              text="有成交后，这里会显示商品收入排名。"
            />
          )}
        </section>
      </div>
      <p className="a-data-time">
        数据更新于{" "}
        {new Date(d.updated_at).toLocaleTimeString("zh-CN", { hour12: false })}{" "}
        · 按支付时间统计，退款回溯原订单 · 金额单位 CNY
      </p>
    </>
  );
}
