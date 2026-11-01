export function paymentLogo(code: string) {
  const logos: Record<string, string> = {
    stripe: "/payments/stripe.svg",
    epay: "/payments/epay.svg",
    dujiaopay: "/payments/dujiaopay.webp",
    bepusdt: "/payments/bepusdt.svg",
  };
  return logos[code] || logos.epay;
}

export function gatewayValues(config: string): Record<string, string> {
  try {
    const values = JSON.parse(config);
    if (!values || typeof values !== "object" || Array.isArray(values))
      return {};
    return Object.fromEntries(
      Object.entries(values).filter((entry) => typeof entry[1] === "string"),
    ) as Record<string, string>;
  } catch {
    return {};
  }
}

export function defaultPaymentName(
  code: string,
  values: Record<string, string>,
) {
  if (code === "dujiaopay" || code === "bepusdt") return "USDT支付";
  if (code === "epay")
    return (
      (
        { alipay: "支付宝", wxpay: "微信支付", qqpay: "QQ 钱包" } as Record<
          string,
          string
        >
      )[values.type || "alipay"] || "在线支付"
    );
  if (code === "stripe") {
    if (!values.payment_methods || values.payment_methods === "auto")
      return "银行卡 / 支付宝 / 微信";
    const names: Record<string, string> = {
      card: "银行卡",
      alipay: "支付宝",
      wechat_pay: "微信支付",
    };
    return (
      values.payment_methods
        .split(",")
        .map((v) => names[v.trim()])
        .filter(Boolean)
        .join(" / ") || "在线支付"
    );
  }
  return "在线支付";
}
