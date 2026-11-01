package pay

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrIgnored = errors.New("verified event requires no action")
var paymentHTTP = &http.Client{Timeout: 20 * time.Second}

func centsString(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }
func parseCents(s string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) > 2 || len(parts[0]) > 12 || strings.HasPrefix(parts[0], "-") {
		return 0, errors.New("invalid amount")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("invalid amount")
	}
	frac := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			for _, c := range parts[1][2:] {
				if c != '0' {
					return 0, errors.New("fractional cent")
				}
			}
		}
		frac = parts[1] + "00"
		frac = frac[:2]
	}
	n, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return whole*100 + n, nil
}
func md5Signature(params map[string]string, secret, excluded string) string {
	keys := []string{}
	for k, v := range params {
		if k != excluded && k != "sign_type" && v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := []string{}
	for _, k := range keys {
		pairs = append(pairs, k+"="+params[k])
	}
	sum := md5.Sum([]byte(strings.Join(pairs, "&") + secret))
	return hex.EncodeToString(sum[:])
}
func hmacHex(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
func freshTimestamp(timestamp string) bool {
	n, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := time.Now().Unix() - n
	return delta >= -300 && delta <= 300
}
func responseJSON(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("支付服务返回 HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(data, target)
}
