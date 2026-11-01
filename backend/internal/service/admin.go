package service

import (
	"errors"
	"time"

	"github.com/wybo/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// ErrUnauthorized 登录失败
var ErrUnauthorized = errors.New("用户名或密码错误")

// EnsureDefaultAdmin 首次启动播种默认管理员
func (s *Service) EnsureDefaultAdmin(username, password string) error {
	var cnt int64
	s.db.Model(&model.Admin{}).Count(&cnt)
	if cnt > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.db.Create(&model.Admin{
		Username: username, PasswordHash: string(hash), Role: model.AdminRoleSuper,
	}).Error
}

// AuthenticateAdmin 校验管理员账号密码
func (s *Service) AuthenticateAdmin(username, password string) (*model.Admin, error) {
	var a model.Admin
	if err := s.db.Where("username = ?", username).First(&a).Error; err != nil {
		return nil, ErrUnauthorized
	}
	if a.Disabled {
		return nil, ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword([]byte(a.PasswordHash), []byte(password)) != nil {
		return nil, ErrUnauthorized
	}
	return &a, nil
}

// EnsureDefaultGateways 播种 5 个支付网关（默认停用，后台启用后填密钥）
func (s *Service) EnsureDefaultGateways() error {
	seeds := []struct{ code, name string }{
		{model.GatewayEpay, "易支付"},
		{model.GatewayStripe, "Stripe"},
		{model.GatewayDujiao, "DujiaoPay"},
		{model.GatewayBepusdt, "BEpusdt"},
		{model.GatewayAlipay, "支付宝官方"},
		{model.GatewayWechat, "微信官方"},
		{model.GatewayUsdt, "USDT"},
	}
	for i, sd := range seeds {
		g := model.Gateway{Code: sd.code, Name: sd.name, Enabled: false, Sort: i, Config: "{}"}
		if err := s.db.Where("code = ?", sd.code).FirstOrCreate(&g).Error; err != nil {
			return err
		}
	}
	return nil
}

// GetGateway 读取已启用的支付网关配置
func (s *Service) GetGateway(code string) (*model.Gateway, error) { return s.loadGateway(code, true) }

// Callbacks remain verifiable when a provider is disabled for new checkouts.
func (s *Service) CallbackGateway(code string) (*model.Gateway, error) {
	return s.loadGateway(code, false)
}
func (s *Service) loadGateway(code string, enabledOnly bool) (*model.Gateway, error) {
	var g model.Gateway
	q := s.db.Where("code = ?", code)
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	if err := q.First(&g).Error; err != nil {
		return nil, err
	}
	plain, err := s.unseal(g.Config)
	if err != nil {
		return nil, err
	}
	g.Config = plain
	return &g, nil
}

// Dashboard 后台仪表盘统计
func (s *Service) Dashboard() (map[string]interface{}, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	paid := []string{model.OrderPaid, model.OrderCompleted}

	var todayOrders, todayRevenue, pendingOrders int64
	s.db.Model(&model.Order{}).Where("created_at >= ?", start).Count(&todayOrders)
	s.db.Model(&model.Order{}).
		Where("paid_at >= ? AND status IN ?", start, paid).
		Select("COALESCE(SUM(total_cents),0)").Scan(&todayRevenue)
	s.db.Model(&model.Order{}).Where("status = ?", model.OrderPending).Count(&pendingOrders)

	var lowStock int64
	s.db.Model(&model.Product{}).Where("stock <= 10").Count(&lowStock)

	return map[string]interface{}{
		"today_orders":        todayOrders,
		"today_revenue_cents": todayRevenue,
		"pending_orders":      pendingOrders,
		"low_stock_products":  lowStock,
	}, nil
}

// ListOrders 订单列表
func (s *Service) ListOrders(status string, page, size int) ([]model.Order, int64, error) {
	var list []model.Order
	var total int64
	q := s.db.Model(&model.Order{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}
