package mq

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestExtractRetryCount(t *testing.T) {
	tests := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{name: "nil", headers: nil, want: 0},
		{name: "missing", headers: amqp.Table{"other": 1}, want: 0},
		{name: "int", headers: amqp.Table{"x-retry-count": 2}, want: 2},
		{name: "int32", headers: amqp.Table{"x-retry-count": int32(3)}, want: 3},
		{name: "int64", headers: amqp.Table{"x-retry-count": int64(4)}, want: 4},
		{name: "float64", headers: amqp.Table{"x-retry-count": float64(5)}, want: 5},
		{name: "string", headers: amqp.Table{"x-retry-count": "6"}, want: 6},
		{name: "bad string", headers: amqp.Table{"x-retry-count": "x"}, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractRetryCount(tc.headers)
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestCloneHeaders(t *testing.T) {
	in := amqp.Table{"x-retry-count": int32(1), "x-last-error": "boom"}
	out := cloneHeaders(in)

	out["x-retry-count"] = int32(7)
	if in["x-retry-count"].(int32) != 1 {
		t.Fatalf("expected source header unchanged")
	}
}
