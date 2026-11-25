import { useEffect, useState } from "react";
import { ArrowUpRight, Copy, Headphones } from "lucide-react";
import { useSite } from "./Site";

export const supportChannels = [
  { key: "support_qq_group", label: "QQ 群号", hint: "填写数字群号，顾客可复制后在 QQ 中搜群。" },
  { key: "support_qq", label: "QQ 号", hint: "填写客服的 QQ 号码。" },
  { key: "support_telegram", label: "Telegram", hint: "填写 @用户名或 t.me 用户名链接。", chatUrl: "https://t.me/" },
  { key: "support_whatsapp", label: "WhatsApp", hint: "填写含国家或地区码的号码，或对应的 wa.me 链接。", chatUrl: "https://wa.me/" },
] as const;

export function ContactButton({ onClick }: { onClick: () => void }) {
  return (
    <button className="header-support" type="button" aria-haspopup="dialog" onClick={onClick}>
      <Headphones size={17} aria-hidden="true" />
      联系客服
    </button>
  );
}

export function SupportContacts({ placement }: { placement: "storefront" | "dialog" | "payment" }) {
  const { site } = useSite();
  const [feedback, setFeedback] = useState("");
  const channels = supportChannels.filter(({ key }) => site[key]?.trim());
  const note = site.customer_service?.trim();
  useEffect(() => {
    if (!feedback) return;
    const timeout = window.setTimeout(() => setFeedback(""), 3200);
    return () => window.clearTimeout(timeout);
  }, [feedback]);

  if (!channels.length && !note) {
    return placement === "dialog" ? (
      <p className="muted">店主暂未公布客服联系方式。你可以查询订单或查看购买指南。</p>
    ) : null;
  }

  async function copyNumber(label: string, value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setFeedback(label + "已复制");
    } catch {
      setFeedback("请选中号码后手动复制。");
    }
  }

  const methods = channels.length > 0 && (
    <ul className="support-methods">
      {channels.map((channel) => {
        const { key, label } = channel;
        const value = site[key].trim();
        const account = (
          <span className="support-account">
            <small>{label}</small>
            <strong dir="ltr">{value}</strong>
          </span>
        );
        return (
          <li key={key}>
            {"chatUrl" in channel ? (
              <a
                className="support-method"
                href={channel.chatUrl + encodeURIComponent(value.replace(/^[@+]/, ""))}
                target="_blank"
                rel="noopener noreferrer"
              >
                {account}
                <ArrowUpRight size={17} aria-hidden="true" />
              </a>
            ) : (
              <div className="support-method">
                {account}
                <button
                  type="button"
                  onClick={() => void copyNumber(label, value)}
                  aria-label={"复制 " + label}
                  title={"复制 " + label}
                >
                  <Copy size={16} aria-hidden="true" />
                </button>
              </div>
            )}
          </li>
        );
      })}
    </ul>
  );
  const content = (
    <div className="support-content">
      {placement === "payment" && methods ? (
        <details className="support-payment">
          <summary>查看客服联系方式</summary>
          {methods}
        </details>
      ) : methods}
      {note && <p className="support-contact-note">{note}</p>}
      <p className="support-feedback" role="status">{feedback}</p>
    </div>
  );
  if (placement === "storefront") {
    return (
      <section className="container support-strip" aria-label="店铺客服">
        <h2><Headphones size={19} aria-hidden="true" />店铺客服</h2>
        {content}
      </section>
    );
  }
  return content;
}
