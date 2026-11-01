package service

import (
	"errors"
	"github.com/wybo/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

func (s *Service) VerifyCustomer(no, contact string) (*model.Order, error) {
	o, err := s.GetOrder(no)
	if err != nil || contact == "" || o.Contact != contact {
		return nil, errors.New("订单或查询凭证不正确")
	}
	return o, nil
}
func validMessage(text string) bool { return len(strings.TrimSpace(text)) >= 2 && len(text) <= 4000 }
func (s *Service) CreateTicket(no, contact, subject, content string) (*model.Ticket, error) {
	o, err := s.VerifyCustomer(no, contact)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(subject)) < 2 || len(subject) > 120 || !validMessage(content) {
		return nil, errors.New("主题为 2–120 字节，内容为 2–4000 字节")
	}
	var count int64
	s.db.Model(&model.Ticket{}).Where("order_id = ? AND status != ?", o.ID, "resolved").Count(&count)
	if count >= 5 {
		return nil, errors.New("此订单尚有未解决工单，请在原工单中回复")
	}
	t := &model.Ticket{Number: "T" + randomID()[:18], OrderID: o.ID, OrderNo: o.OrderNo, Subject: strings.TrimSpace(subject), Status: "open"}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(t).Error; err != nil {
			return err
		}
		return tx.Create(&model.TicketMessage{TicketID: t.ID, Author: "customer", Content: strings.TrimSpace(content)}).Error
	})
	return t, err
}
func (s *Service) CustomerTickets(no, contact string) ([]map[string]interface{}, error) {
	o, err := s.VerifyCustomer(no, contact)
	if err != nil {
		return nil, err
	}
	var tickets []model.Ticket
	if err = s.db.Where("order_id = ?", o.ID).Order("id DESC").Find(&tickets).Error; err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	for _, t := range tickets {
		var messages []model.TicketMessage
		s.db.Where("ticket_id = ? AND internal = ?", t.ID, false).Order("id ASC").Find(&messages)
		out = append(out, map[string]interface{}{"ticket": t, "messages": messages})
	}
	return out, nil
}
func (s *Service) ReplyTicket(number, content, status string, internal bool, admin bool, no, contact string) error {
	if !validMessage(content) {
		return errors.New("回复为 2–4000 字节")
	}
	if !admin {
		if _, err := s.VerifyCustomer(no, contact); err != nil {
			return err
		}
		internal = false
		status = "open"
	}
	if status != "open" && status != "waiting" && status != "resolved" {
		return errors.New("工单状态无效")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var t model.Ticket
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("number = ?", number)
		if !admin {
			q = q.Where("order_no = ?", no)
		}
		if q.First(&t).Error != nil {
			return errors.New("工单不存在")
		}
		author := "customer"
		if admin {
			author = "merchant"
		}
		if err := tx.Create(&model.TicketMessage{TicketID: t.ID, Author: author, Content: strings.TrimSpace(content), Internal: internal}).Error; err != nil {
			return err
		}
		return tx.Model(&t).Update("status", status).Error
	})
}
func (s *Service) AdminTickets(number string) (interface{}, error) {
	if number == "" {
		var rows []model.Ticket
		err := s.db.Order("updated_at DESC").Limit(200).Find(&rows).Error
		return rows, err
	}
	var t model.Ticket
	if s.db.Where("number = ?", number).First(&t).Error != nil {
		return nil, errors.New("工单不存在")
	}
	var messages []model.TicketMessage
	err := s.db.Where("ticket_id = ?", t.ID).Order("id ASC").Find(&messages).Error
	return map[string]interface{}{"ticket": t, "messages": messages}, err
}
