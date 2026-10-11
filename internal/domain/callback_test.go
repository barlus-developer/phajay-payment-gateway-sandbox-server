package domain

import (
	"encoding/json"
	"testing"
)

func TestDistinctCallbackContractsAndTransitionTime(t *testing.T) {
	tx := Transaction{ID: "txn", ProjectID: "project", OrderNo: "order", Kind: "LINK", Bank: "JDB", AmountMinor: 1234, Currency: "LAK", Description: "Test", Status: Failed, CreatedAt: 1700000000}
	raw, e := CallbackBodyAt(tx, 1700000060)
	if e != nil {
		t.Fatal(e)
	}
	var link map[string]any
	if e = json.Unmarshal(raw, &link); e != nil {
		t.Fatal(e)
	}
	if link["amount"] != 12.34 || link["descriptione"] != "Test" || link["user"] != "project" || link["updateAt"] != "2023-11-14T22:14:20Z" || link["txnAmount"] != nil {
		t.Fatal(link)
	}
	tx.Kind = "QR"
	tx.Status = Completed
	completed := int64(1700000060)
	tx.CompletedAt = &completed
	raw, e = CallbackBody(tx)
	if e != nil {
		t.Fatal(e)
	}
	var qr map[string]any
	json.Unmarshal(raw, &qr)
	if qr["txnAmount"] != 12.34 || qr["transactionId"] != "txn" || qr["paymentMethod"] != "JDB" || qr["description"] != "Test" || qr["amount"] != nil {
		t.Fatal(qr)
	}
	for state, want := range map[State]string{Completed: "payment.completed", Failed: "payment.failed", Canceled: "payment.canceled", Expired: "payment.expired", Waiting: "payment.waiting"} {
		if got := EventType(state); got != want {
			t.Fatal(state, got)
		}
	}
}
