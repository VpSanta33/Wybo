import {
  ArrowUpRight,
  Copy,
  LoaderCircle,
  MessageCircle,
  RefreshCw,
  Sparkles,
} from "lucide-react";
import type { ShopOrder } from "./api";
import { OrderSummary, PaymentAction } from "./components";
import SupportTickets from "./SupportTickets";

function redeemLink(value: string) {
  try {
    const url = new URL(value);
    return ["https:", "http:"].includes(url.protocol) &&
      !url.username &&
      !url.password
      ? url
      : null;
  } catch {
    return null;
  }
}

export default function OrderResult({
  order,
  onCopy,
  onRefresh,
  onCancel,
  onViewOrder,
  busy,
  cancelBusy,
}: {
  order: ShopOrder;
  onCopy: (value: string) => void;
  onRefresh: () => void;
  onCancel: () => void;
  onViewOrder: (orderNo: string) => void;
  busy: boolean;
  cancelBusy: boolean;
}) {
  const manual = order.items?.some((item) => item.delivery_mode === "manual");
  const upgraded = order.after_sales?.some(
    (v) => v.kind === "upgrade" && v.status === "completed",
  );
  return (
    <>
      <OrderSummary order={order} onCopy={onCopy} compact />
      {order.status === "completed" && (
        <>
          <div className="delivered-cards">
            <h3>
              {manual ? "你的商品 / 发货内容" : "你的卡密"}{" "}
              <span>点击右侧复制</span>
            </h3>
            {order.cards?.length ? (
              <div className="delivered-card-list">
                {order.cards.map((card, index) => (
                  <div className="delivered-card" key={index}>
                    <code>
                      {card.code}
                      {card.secret ? `\n密码：${card.secret}` : ""}
                    </code>
                    <button
                      className="icon-button"
                      type="button"
                      onClick={() =>
                        onCopy(
                          `${card.code}${card.secret ? `\n${card.secret}` : ""}`,
                        )
                      }
                      aria-label={
                        manual
                          ? `复制第${index + 1}份发货内容`
                          : `复制第${index + 1}张卡密`
                      }
                    >
                      <Copy size={17} />
                    </button>
                  </div>
                ))}
              </div>
            ) : (
              <p>
                {upgraded
                  ? "原卡已升级，请查看补差订单获取新卡密。"
                  : "卡密正在准备中，请稍后刷新订单状态。"}
              </p>
            )}
          </div>
          {!!order.cards?.length && (
            <section className="redemption-section" aria-label="兑换与使用">
              <h3>下一步：兑换与使用</h3>
              <p>
                {manual
                  ? "请按发货内容和商品说明使用；需要兑换的卡密可通过对应入口操作。"
                  : "复制卡密，打开对应兑换入口，按说明完成兑换。"}
              </p>
              {order.redemptions?.length ? (
                order.redemptions.map((guide, i) => {
                  const url = redeemLink(guide.redeem_url);
                  return (
                    <article
                      className="redemption-guide"
                      key={`${guide.product_id}-${i}`}
                    >
                      <div className="redemption-heading">
                        <strong>{guide.product_name}</strong>
                        {url && (
                          <a
                            className="button button-primary"
                            href={url.href}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            前往兑换
                            <ArrowUpRight size={16} aria-hidden="true" />
                            <span className="sr-only">（在新标签页打开）</span>
                          </a>
                        )}
                      </div>
                      {url && (
                        <span className="redemption-domain">
                          兑换平台：{url.hostname}
                        </span>
                      )}
                      {guide.redeem_guide &&
                        (guide.redeem_guide.length > 240 ? (
                          <details>
                            <summary>查看详细兑换步骤</summary>
                            <p>{guide.redeem_guide}</p>
                          </details>
                        ) : (
                          <p>{guide.redeem_guide}</p>
                        ))}
                      {!url && (
                        <p className="redemption-missing">
                          店主尚未设置兑换链接，请按兑换说明操作；需要帮助可在下方联系订单售后。
                        </p>
                      )}
                    </article>
                  );
                })
              ) : (
                <p>
                  店主尚未提供兑换说明，请通过下方“订单售后”联系店主获取兑换入口。
                </p>
              )}
            </section>
          )}
        </>
      )}
      {order.status === "paid" && (
        <p className="soft-note" role="status">
          <LoaderCircle size={17} className="spin" />
          {manual
            ? "支付成功，等待店主手工发货。发货内容会在这里显示，可通过订单售后联系店主。"
            : "支付成功，正在发放卡密。发货后会在这里显示卡密和兑换入口。"}
        </p>
      )}
      {order.after_sales
        ?.filter((v) => v.kind === "upgrade")
        .map((v) => (
          <div className="soft-note" key={v.child_order_no}>
            <span>
              {v.status === "completed"
                ? "已完成升级；原卡已被替换。"
                : v.status === "canceled"
                  ? "升级补差订单已关闭。"
                  : "有一笔升级补差订单等待付款。"}
            </span>
            <button
              className="text-button"
              disabled={busy}
              onClick={() => onViewOrder(v.child_order_no)}
            >
              查看补差订单
            </button>
          </div>
        ))}
      {order.after_sales
        ?.filter((v) => v.kind !== "upgrade")
        .map((v, i) => (
          <p className="soft-note" key={`${v.kind}-${i}`}>
            {v.kind === "replacement"
              ? "此订单已换发卡密，上方为当前有效卡密。"
              : "此订单已登记售后退款。"}
          </p>
        ))}
      {order.status === "preview" && (
        <p className="soft-note">
          <Sparkles size={17} />
          演示订单仅用于体验，不产生付款或真实卡密。
        </p>
      )}
      {order.status === "pending" && <PaymentAction order={order} />}
      {order.status === "closed" && (
        <p className="soft-note">
          此订单已关闭，无需继续付款。如需购买，请返回小铺重新下单。
        </p>
      )}
      {order.status !== "preview" && (
        <div className="order-result-actions">
          <button
            className="text-button"
            onClick={onRefresh}
            disabled={busy || cancelBusy}
          >
            <RefreshCw size={15} className={busy ? "spin" : ""} />
            {busy ? "刷新中…" : "刷新订单"}
          </button>
          {order.status === "pending" && (
            <button
              className="cancel-order"
              onClick={onCancel}
              disabled={cancelBusy || busy}
            >
              {cancelBusy ? "正在取消…" : "取消订单"}
            </button>
          )}
        </div>
      )}
      {order.status !== "preview" && (
        <details className="order-support" key={order.order_no}>
          <summary>
            <MessageCircle size={17} aria-hidden="true" />
            需要帮助？订单售后
          </summary>
          <SupportTickets
            orderNo={order.order_no}
            contact={order.contact || ""}
          />
        </details>
      )}
    </>
  );
}
