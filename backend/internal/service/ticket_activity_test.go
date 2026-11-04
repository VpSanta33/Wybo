package service

import (
	"testing"

	"github.com/wybo/backend/internal/model"
)

func TestTicketActivityCustomerMessagesOnly(t *testing.T) {
	s, db, p := operationsFixture(t)
	empty, err := s.TicketActivity(nil)
	must(t, err)
	if empty.Cursor != 0 || empty.Count != 0 {
		t.Fatal("empty activity should establish an empty baseline", empty)
	}
	order, err := s.CreateOrder(CreateOrderReq{ProductID: p.ID, Quantity: 1, Contact: "buyer@example.test"})
	must(t, err)
	ticket, err := s.CreateTicket(order.OrderNo, order.Contact, "需要售后帮助", "请检查兑换状态")
	must(t, err)
	baseline, err := s.TicketActivity(nil)
	must(t, err)
	if baseline.Cursor == 0 || baseline.Count != 0 {
		t.Fatal("initial request replayed historical messages", baseline)
	}
	must(t, s.ReplyTicket(ticket.Number, "正在处理", "waiting", false, true, "", ""))
	must(t, s.ReplyTicket(ticket.Number, "内部排查记录", "open", true, true, "", ""))
	merchant, err := s.TicketActivity(&baseline.Cursor)
	must(t, err)
	if merchant.Count != 0 || merchant.Cursor != baseline.Cursor {
		t.Fatal("merchant replies or internal notes triggered an alert", merchant)
	}
	// Follow-ups must alert even if the existing ticket status remains open.
	must(t, s.ReplyTicket(ticket.Number, "顾客再次追问", "open", false, false, order.OrderNo, order.Contact))
	must(t, s.ReplyTicket(ticket.Number, "补充说明问题", "open", false, false, order.OrderNo, order.Contact))
	fresh, err := s.TicketActivity(&baseline.Cursor)
	must(t, err)
	if fresh.Count != 2 || fresh.Cursor <= baseline.Cursor {
		t.Fatal("customer follow-ups were missed", fresh)
	}
	unchanged, err := s.TicketActivity(&fresh.Cursor)
	must(t, err)
	if unchanged.Count != 0 || unchanged.Cursor != fresh.Cursor {
		t.Fatal("unchanged polling repeated an alert", unchanged)
	}
	// Never report customer-tagged internal content or its text through this API.
	must(t, db.Create(&model.TicketMessage{TicketID: ticket.ID, Author: "customer", Internal: true, Content: "private-internal-test"}).Error)
	internal, err := s.TicketActivity(&fresh.Cursor)
	must(t, err)
	if internal.Count != 0 || internal.Cursor != fresh.Cursor {
		t.Fatal("internal message triggered an alert", internal)
	}
	zero := uint(0)
	all, err := s.TicketActivity(&zero)
	must(t, err)
	if all.Count != 3 {
		t.Fatal("new tickets and follow-ups should all be counted", all)
	}
	// A stale cursor after a database restore resets without replaying history.
	ahead := fresh.Cursor + 100
	reset, err := s.TicketActivity(&ahead)
	must(t, err)
	if reset.Count != 0 || reset.Cursor != fresh.Cursor {
		t.Fatal("stale cursor did not reset", reset)
	}
}
