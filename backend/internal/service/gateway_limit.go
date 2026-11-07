package service

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 单笔限额：渠道配置中 limit_enabled = "true" 时，max_amount（元）为每笔订单可支付的最高金额。
// 例如牛牛支付单笔不能超过 500 元，超出的订单需由顾客联系客服下单。
const (
	gatewayLimitEnabledKey = "limit_enabled"
	gatewayMaxAmountKey    = "max_amount"
	maxGatewayLimitCents   = int64(100000000) // ¥1,000,000
)

var yuanAmount = regexp.MustCompile(`^\d{1,7}(\.\d{1,2})?$`)

// GatewayLimitError 表示订单金额超过渠道单笔限额。
type GatewayLimitError struct{ LimitCents int64 }

func (e *GatewayLimitError) Error() string {
	return fmt.Sprintf("该支付方式单笔限额 ¥%s，请更换支付方式或联系客服下单", centsYuan(e.LimitCents))
}

// ErrGatewayAmountLimit 用于 errors.Is 判断。
var ErrGatewayAmountLimit = errors.New("gateway amount limit")

func (e *GatewayLimitError) Is(target error) bool { return target == ErrGatewayAmountLimit }

func parseYuanCents(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if !yuanAmount.MatchString(v) {
		return 0, errors.New("invalid amount")
	}
	whole, frac, _ := strings.Cut(v, ".")
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	f, _ := strconv.ParseInt((frac + "00")[:2], 10, 64)
	return w*100 + f, nil
}

func centsYuan(c int64) string {
	if c%100 == 0 {
		return strconv.FormatInt(c/100, 10)
	}
	return fmt.Sprintf("%d.%02d", c/100, c%100)
}

// gatewayLimitCents 返回渠道单笔限额（分），0 表示不限制。
func gatewayLimitCents(cfg map[string]string) int64 {
	if cfg[gatewayLimitEnabledKey] != "true" {
		return 0
	}
	v, err := parseYuanCents(cfg[gatewayMaxAmountKey])
	if err != nil || v <= 0 {
		return 0
	}
	return v
}

func validateGatewayLimit(cfg map[string]string) error {
	switch cfg[gatewayLimitEnabledKey] {
	case "", "false":
		return nil
	case "true":
	default:
		return errors.New("单笔限额开关无效")
	}
	v, err := parseYuanCents(cfg[gatewayMaxAmountKey])
	if err != nil || v <= 0 || v > maxGatewayLimitCents {
		return errors.New("单笔限额须为大于 0 的金额（元），最多两位小数")
	}
	cfg[gatewayMaxAmountKey] = centsYuan(v)
	return nil
}
