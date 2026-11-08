package main

import (
	"testing"
	"unsafe"

	"github.com/BergerAPI/go-fints"
)

func TestTypedTransactionArrayOwnsValuesAndCanBeFreed(t *testing.T) {
	array := transactionArrayToC([]fints.Transaction{{
		BookingDate: "2026-07-23",
		Amount:      -1234,
		Currency:    "EUR",
		Description: "Typed C transaction",
	}})
	if int(array.length) != 1 || array.items == nil {
		t.Fatalf("array = %#v", array)
	}
	items := unsafe.Slice(array.items, int(array.length))
	if int64(items[0].amount) != -1234 ||
		goString(items[0].description) != "Typed C transaction" {
		t.Fatalf("transaction = %#v", items[0])
	}

	FintsTransactionArrayFree(&array)
	if array.items != nil || array.length != 0 {
		t.Fatalf("freed array = %#v", array)
	}
	// Every free function is intentionally idempotent for zeroed results.
	FintsTransactionArrayFree(&array)
}

func TestTypedAccountArrayIncludesSortedSupportedOperations(t *testing.T) {
	array := accountArrayToC([]fints.Account{{
		IBAN: "DE89370400440532013000",
		SupportedOperations: map[string]bool{
			"HKCCS": true,
			"HKKAZ": true,
			"HKCAZ": false,
		},
	}})
	items := unsafe.Slice(array.items, int(array.length))
	operations := goStringArray(
		items[0].supported_operations,
		items[0].supported_operation_count,
	)
	if len(operations) != 2 ||
		operations[0] != "HKCCS" ||
		operations[1] != "HKKAZ" {
		t.Fatalf("supported operations = %#v", operations)
	}
	FintsAccountArrayFree(&array)
}

func TestTypedCapabilityArrayIncludesNestedArrays(t *testing.T) {
	array := capabilityArrayToC([]fints.Capability{{
		Operation:   "HKKAZ",
		Versions:    []int{5, 6, 7},
		Formats:     []string{"MT940"},
		TANRequired: true,
		MaxDays:     90,
	}})
	items := unsafe.Slice(array.items, int(array.length))
	versions := unsafe.Slice(items[0].versions, int(items[0].version_count))
	if int(items[0].version_count) != 3 ||
		int(versions[2]) != 7 ||
		goStringArray(items[0].formats, items[0].format_count)[0] != "MT940" ||
		items[0].tan_required != 1 ||
		items[0].max_days != 90 {
		t.Fatalf("capability = %#v", items[0])
	}
	FintsCapabilityArrayFree(&array)
}
