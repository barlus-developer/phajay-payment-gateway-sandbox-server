package domain

import (
	"encoding/json"
	"time"
)

func EventType(status State) string {
	switch status {
	case Completed:
		return "payment.completed"
	case Failed:
		return "payment.failed"
	case Canceled:
		return "payment.canceled"
	case Expired:
		return "payment.expired"
	}
	return "payment.waiting"
}

// CallbackBody preserves the SDK's distinct sandbox link and bank QR contracts.
func CallbackBody(t Transaction) ([]byte, error) {
	return CallbackBodyAt(t, t.CreatedAt)
}

// CallbackBodyAt records the transition time for failed/canceled/expired events.
func CallbackBodyAt(t Transaction, updated int64) ([]byte, error) {
	if t.CompletedAt != nil {
		updated = *t.CompletedAt
	}
	date := time.Unix(updated, 0).UTC().Format(time.RFC3339)
	common := map[string]any{"status": string(t.Status), "message": string(t.Status), "tag1": t.Tag1, "tag2": t.Tag2, "tag3": t.Tag3}
	if t.Kind == "LINK" {
		common["_id"] = t.ID
		common["orderNo"] = t.OrderNo
		common["linkCode"] = t.ID
		common["amount"] = AmountNumber(t.AmountMinor)
		common["currency"] = t.Currency
		common["descriptione"] = t.Description
		common["user"] = t.ProjectID
		common["createdAt"] = time.Unix(t.CreatedAt, 0).UTC().Format(time.RFC3339)
		common["updateAt"] = date
		common["isMerchantIdPayment"] = false
		common["isAffiliatePayment"] = false
	} else {
		common["paymentMethod"] = t.Bank
		common["transactionId"] = t.ID
		common["txnAmount"] = AmountNumber(t.AmountMinor)
		common["billNumber"] = t.OrderNo
		common["refNo"] = t.ReceiptRef
		common["merchantName"] = t.MerchantName
		common["description"] = t.Description
		common["txnDateTime"] = date
		common["sourceCurrency"] = t.Currency
		common["userId"] = t.ProjectID
	}
	return json.Marshal(common)
}
