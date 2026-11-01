import { useEffect, useState, type FormEvent } from "react";
import { MessageCircle, Send, Plus, RefreshCw } from "lucide-react";
import { api } from "./api";
import { ErrorMessage } from "./components";
type Thread = {
  ticket: {
    number: string;
    subject: string;
    status: string;
    updated_at: string;
  };
  messages: {
    id: number;
    author: string;
    content: string;
    created_at: string;
  }[];
};
export default function SupportTickets({
  orderNo,
  contact,
}: {
  orderNo: string;
  contact: string;
}) {
  const [threads, setThreads] = useState<Thread[]>([]),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [open, setOpen] = useState(false),
    [subject, setSubject] = useState(""),
    [content, setContent] = useState(""),
    [reply, setReply] = useState(""),
    [active, setActive] = useState("");
  async function load(signal?: AbortSignal) {
    setLoading(true);
    setError("");
    try {
      setThreads(
        await api<Thread[]>(
          `/orders/${encodeURIComponent(orderNo)}/tickets?contact=${encodeURIComponent(contact)}`,
          { signal },
        ),
      );
    } catch (e) {
      if (!signal?.aborted) setError((e as Error).message);
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }
  useEffect(() => {
    const ctrl = new AbortController();
    load(ctrl.signal);
    return () => ctrl.abort();
  }, [orderNo, contact]);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (active) {
        await api(`/orders/${orderNo}/tickets/${active}/replies`, {
          method: "POST",
          body: JSON.stringify({ contact, content: reply }),
        });
        setReply("");
      } else {
        const t = await api<{ number: string }>(`/orders/${orderNo}/tickets`, {
          method: "POST",
          body: JSON.stringify({ contact, subject, content }),
        });
        setActive(t.number);
        setOpen(false);
        setSubject("");
        setContent("");
      }
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="support-tickets">
      <div className="support-heading">
        <h3>
          <MessageCircle size={18} />
          订单售后
        </h3>
        <button
          className="text-button"
          disabled={busy}
          onClick={() => {
            setActive("");
            setOpen(!open);
            setError("");
          }}
        >
          <Plus size={15} />
          提交问题
        </button>
      </div>
      {error && <ErrorMessage>{error}</ErrorMessage>}
      {loading ? (
        <p role="status">正在读取工单…</p>
      ) : threads.length ? (
        <div className="support-list">
          {threads.map((t) => (
            <article key={t.ticket.number}>
              <button
                className="support-ticket-toggle"
                aria-expanded={active === t.ticket.number}
                onClick={() => {
                  setActive(active === t.ticket.number ? "" : t.ticket.number);
                  setReply("");
                  setOpen(false);
                }}
              >
                <strong>{t.ticket.subject}</strong>
                <span>
                  {{
                    open: "处理中",
                    waiting: "等待你的回复",
                    resolved: "已解决",
                  }[t.ticket.status] || t.ticket.status}
                </span>
              </button>
              {active === t.ticket.number && (
                <div className="support-thread">
                  <small>{t.ticket.number}</small>
                  {t.messages.map((m) => (
                    <div
                      className={`support-message ${m.author === "merchant" ? "from-merchant" : ""}`}
                      key={m.id}
                    >
                      <div>
                        <b>
                          {m.author === "merchant" ? "店主回复" : "我的留言"}
                        </b>
                        <time>
                          {new Date(m.created_at).toLocaleString("zh-CN")}
                        </time>
                      </div>
                      <p>{m.content}</p>
                    </div>
                  ))}
                  <form onSubmit={submit}>
                    <label>
                      继续留言
                      <textarea
                        required
                        minLength={2}
                        maxLength={4000}
                        rows={3}
                        value={reply}
                        onChange={(e) => setReply(e.target.value)}
                        placeholder="补充问题或回复店主…"
                      />
                    </label>
                    <button className="button button-primary" disabled={busy}>
                      <Send size={15} />
                      {busy ? "发送中…" : "发送留言"}
                    </button>
                  </form>
                </div>
              )}
            </article>
          ))}
        </div>
      ) : (
        <p className="support-note">
          遇到使用问题，可以在这里联系店主并查看回复。
        </p>
      )}
      {open && (
        <form className="support-new" onSubmit={submit}>
          <label>
            问题主题
            <input
              required
              minLength={2}
              maxLength={120}
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="例如：兑换遇到问题"
            />
          </label>
          <label>
            详细说明
            <textarea
              required
              minLength={2}
              maxLength={4000}
              rows={4}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder="请描述具体问题，勿填写密码或完整卡密。"
            />
          </label>
          <button className="button button-primary" disabled={busy}>
            {busy ? "提交中…" : "提交工单"}
          </button>
        </form>
      )}
      <button
        className="text-button support-refresh"
        disabled={loading || busy}
        onClick={() => load()}
      >
        <RefreshCw size={14} />
        刷新回复
      </button>
    </section>
  );
}
