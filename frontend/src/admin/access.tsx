import {
  createContext,
  useContext,
  useEffect,
  useState,
  type FormEvent,
} from "react";
import { Modal } from "../components";
import { adminApi, type AdminUser } from "./api";
import { Banner, Field } from "./ui";
export const AccessContext = createContext<AdminUser | null>(null);
export function useAccess() {
  const user = useContext(AccessContext);
  return {
    user,
    can: (permission: string) =>
      !!user &&
      (user.permissions.includes("*") || user.permissions.includes(permission)),
  };
}
type Confirmation = {
  action: string;
  resolve: (v: string) => void;
  reject: (e: Error) => void;
};
export function StepUp() {
  const [request, setRequest] = useState<Confirmation | null>(null),
    [password, setPassword] = useState(""),
    [code, setCode] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const { user } = useAccess();
  useEffect(() => {
    let active: Confirmation | null = null;
    const listener = (event: Event) => {
      const next = (event as CustomEvent<Confirmation>).detail;
      if (active) {
        next.reject(new Error("请先完成当前安全验证"));
        return;
      }
      active = next;
      setPassword("");
      setCode("");
      setError("");
      setRequest(next);
    };
    window.addEventListener("wybo-admin-confirm", listener);
    const done = () => {
      active = null;
    };
    window.addEventListener("wybo-admin-confirm-done", done);
    return () => {
      window.removeEventListener("wybo-admin-confirm", listener);
      window.removeEventListener("wybo-admin-confirm-done", done);
      active?.reject(new Error("操作已取消"));
    };
  }, []);
  function close() {
    if (busy) return;
    request?.reject(new Error("已取消安全验证"));
    finish();
  }
  function finish() {
    setRequest(null);
    setPassword("");
    setCode("");
    window.dispatchEvent(new Event("wybo-admin-confirm-done"));
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!request) return;
    setBusy(true);
    setError("");
    try {
      const v = await adminApi<{ token: string }>("/confirm", {
        method: "POST",
        body: JSON.stringify({ password, code, action: request.action }),
      });
      request.resolve(v.token);
      finish();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return request ? (
    <Modal title="验证后继续" onClose={close}>
      <form className="a-editor" onSubmit={submit}>
        <h2>验证后继续</h2>
        <p>此操作涉及敏感数据或资金记录，请验证当前管理员身份。</p>
        {error && <Banner>{error}</Banner>}
        <Field label="当前密码">
          <input
            required
            autoFocus
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        {user?.two_factor_enabled && (
          <Field label="两步验证码">
            <input
              required
              autoComplete="one-time-code"
              inputMode="numeric"
              pattern="[0-9]{6}"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
        )}
        <div className="a-form-actions">
          <button
            type="button"
            className="a-button"
            disabled={busy}
            onClick={close}
          >
            取消
          </button>
          <button className="a-button primary" disabled={busy}>
            {busy ? "正在验证…" : "验证并继续"}
          </button>
        </div>
      </form>
    </Modal>
  ) : null;
}
