package main

/*
#include <stdint.h>
#include <stddef.h>
#include <stdlib.h>

typedef uint64_t FintsClientHandle;

enum {
	FINTS_OK = 0,
	FINTS_ERROR_INVALID_ARGUMENT = 1,
	FINTS_ERROR_INVALID_HANDLE = 2,
	FINTS_ERROR_OPERATION = 3,
	FINTS_ERROR_BANK = 4,
	FINTS_ERROR_TAN_REQUIRED = 5
};

typedef struct {
	int32_t code;
	char *message;
} FintsError;

typedef struct {
	char *method;
	char *medium;
	char *process;
	int32_t hktan_version;
	char *task_reference;
	char *challenge;
	int32_t decoupled;
	int32_t poll_attempt;
} FintsTANChallenge;

typedef char* (*FintsTANCallback)(
	const FintsTANChallenge *challenge,
	void *user_data
);

typedef struct {
	char *iban;
	char *bic;
	char *account_number;
	char *subaccount;
	char *country_code;
	char *bank_code;
	char *currency;
	char *type;
	char *owner_name;
	char *product_name;
	char **supported_operations;
	size_t supported_operation_count;
} FintsAccount;

typedef struct {
	int32_t skip_transaction_sepa_account_discovery;
	FintsAccount transaction_account_defaults;
	char **force_tan_for_operations;
	size_t force_tan_for_operation_count;
	char **preferred_security_functions;
	size_t preferred_security_function_count;
	char **tan_medium_required_for_functions;
	size_t tan_medium_required_for_function_count;
	char *decoupled_poll_process;
} FintsCompatibilityOptions;

typedef struct {
	char *endpoint;
	char *country_code;
	char *bank_code;
	char *user_id;
	char *customer_id;
	char *pin;
	char *product_id;
	char *product_version;
	char *system_id;
	char *security_function;
	char *tan_medium;
	int32_t hktan_version;
	int32_t auto_poll;
	int64_t poll_interval_ms;
	int32_t max_polls;
	int64_t timeout_ms;
	int32_t allow_http;
	FintsCompatibilityOptions compatibility;
	FintsTANCallback tan_callback;
	void *tan_user_data;
} FintsConfig;

typedef struct {
	FintsAccount *items;
	size_t length;
} FintsAccountArray;

typedef struct {
	char *booking_date;
	char *value_date;
	int64_t amount;
	char *currency;
	char *type;
	char *customer_reference;
	char *bank_reference;
	char *counterparty_name;
	char *counterparty_iban;
	char *counterparty_bank_id;
	char *description;
} FintsTransaction;

typedef struct {
	FintsTransaction *items;
	size_t length;
} FintsTransactionArray;

typedef struct {
	char *operation;
	int32_t *versions;
	size_t version_count;
	char **formats;
	size_t format_count;
	int32_t tan_required;
	int32_t max_days;
} FintsCapability;

typedef struct {
	FintsCapability *items;
	size_t length;
} FintsCapabilityArray;

typedef struct {
	char *security_function;
	char *name;
	int32_t hktan_version;
	char *tan_process;
	int32_t medium_required;
} FintsTANMethod;

typedef struct {
	FintsTANMethod *items;
	size_t length;
} FintsTANMethodArray;

typedef struct {
	char *name;
	char *class_name;
	char *status;
	char *security_function;
	char *mobile_number;
} FintsTANMedium;

typedef struct {
	FintsTANMedium *items;
	size_t length;
} FintsTANMediumArray;

typedef struct {
	char *code;
	char *message;
	int32_t segment;
	char *reference_element;
	char **params;
	size_t param_count;
} FintsResponseMessage;

typedef struct {
	FintsAccount debtor;
	char *debtor_name;
	char *creditor_name;
	char *creditor_iban;
	char *creditor_bic;
	int64_t amount;
	char *currency;
	char *purpose;
	char *end_to_end_reference;
	char *execution_date;
} FintsSEPACreditTransferRequest;

typedef struct {
	int32_t accepted;
	char *operation;
	char *pain_descriptor;
	FintsResponseMessage *responses;
	size_t response_count;
} FintsTransferResult;

extern char* fints_call_tan_callback(
	FintsTANCallback callback,
	const FintsTANChallenge *challenge,
	void *user_data
);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/BergerAPI/go-fints"
)

const abiVersion = 1

var (
	clientSequence atomic.Uint64
	clientRegistry sync.Map
)

// FintsABIVersion returns the version of the typed C ABI.
//
//export FintsABIVersion
func FintsABIVersion() C.uint32_t {
	return abiVersion
}

// FintsClientCreate validates a typed configuration and creates an opaque
// client handle. The caller owns errors returned through outError.
//
//export FintsClientCreate
func FintsClientCreate(
	config *C.FintsConfig,
	outHandle *C.FintsClientHandle,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if config == nil || outHandle == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"config and out_handle must not be NULL",
		))
	}
	*outHandle = 0

	goConfig, err := configFromC(config)
	if err != nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, err)
	}
	client, err := fints.NewClient(goConfig)
	if err != nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, err)
	}
	handle := clientSequence.Add(1)
	clientRegistry.Store(handle, client)
	*outHandle = C.FintsClientHandle(handle)
	return C.FINTS_OK
}

// FintsClientDestroy releases an opaque client handle.
//
//export FintsClientDestroy
func FintsClientDestroy(handle C.FintsClientHandle) {
	clientRegistry.Delete(uint64(handle))
}

// FintsClientSetTANMedium changes the selected TAN medium on an existing
// client.
//
//export FintsClientSetTANMedium
func FintsClientSetTANMedium(
	handle C.FintsClientHandle,
	medium *C.char,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	client.SetTANMedium(goString(medium))
	return C.FINTS_OK
}

// FintsAccounts returns an owned typed account array.
//
//export FintsAccounts
func FintsAccounts(
	handle C.FintsClientHandle,
	out *C.FintsAccountArray,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if out == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"out must not be NULL",
		))
	}
	*out = C.FintsAccountArray{}
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	accounts, err := client.Accounts(context.Background())
	if err != nil {
		return failOperation(outError, err)
	}
	*out = accountArrayToC(accounts)
	return C.FINTS_OK
}

// FintsTransactions returns an owned typed transaction array. Dates are
// optional YYYY-MM-DD strings; NULL and empty strings mean no bound.
//
//export FintsTransactions
func FintsTransactions(
	handle C.FintsClientHandle,
	account *C.FintsAccount,
	fromDate *C.char,
	toDate *C.char,
	out *C.FintsTransactionArray,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if account == nil || out == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"account and out must not be NULL",
		))
	}
	*out = C.FintsTransactionArray{}
	from, err := parseDate(fromDate)
	if err != nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, fmt.Errorf(
			"from_date: %w",
			err,
		))
	}
	to, err := parseDate(toDate)
	if err != nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, fmt.Errorf(
			"to_date: %w",
			err,
		))
	}
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	transactions, err := client.Transactions(
		context.Background(),
		accountFromC(account),
		from,
		to,
	)
	if err != nil {
		return failOperation(outError, err)
	}
	*out = transactionArrayToC(transactions)
	return C.FINTS_OK
}

// FintsCapabilities returns an owned typed capability array.
//
//export FintsCapabilities
func FintsCapabilities(
	handle C.FintsClientHandle,
	out *C.FintsCapabilityArray,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if out == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"out must not be NULL",
		))
	}
	*out = C.FintsCapabilityArray{}
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	capabilities, err := client.Capabilities(context.Background())
	if err != nil {
		return failOperation(outError, err)
	}
	*out = capabilityArrayToC(capabilities)
	return C.FINTS_OK
}

// FintsTANMethods returns an owned typed TAN-method array.
//
//export FintsTANMethods
func FintsTANMethods(
	handle C.FintsClientHandle,
	out *C.FintsTANMethodArray,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if out == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"out must not be NULL",
		))
	}
	*out = C.FintsTANMethodArray{}
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	methods, err := client.TANMethods(context.Background())
	if err != nil {
		return failOperation(outError, err)
	}
	*out = tanMethodArrayToC(methods)
	return C.FINTS_OK
}

// FintsTANMedia returns an owned typed TAN-medium array.
//
//export FintsTANMedia
func FintsTANMedia(
	handle C.FintsClientHandle,
	out *C.FintsTANMediumArray,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if out == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"out must not be NULL",
		))
	}
	*out = C.FintsTANMediumArray{}
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	media, err := client.TANMedia(context.Background())
	if err != nil {
		return failOperation(outError, err)
	}
	*out = tanMediumArrayToC(media)
	return C.FINTS_OK
}

// FintsSEPACreditTransfer submits a typed SEPA credit transfer and returns an
// owned typed result. This performs a real state-changing bank operation.
//
//export FintsSEPACreditTransfer
func FintsSEPACreditTransfer(
	handle C.FintsClientHandle,
	transfer *C.FintsSEPACreditTransferRequest,
	out *C.FintsTransferResult,
	outError **C.FintsError,
) C.int32_t {
	clearError(outError)
	if transfer == nil || out == nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, errors.New(
			"transfer and out must not be NULL",
		))
	}
	*out = C.FintsTransferResult{}
	executionDate, err := parseDate(transfer.execution_date)
	if err != nil {
		return fail(outError, C.FINTS_ERROR_INVALID_ARGUMENT, fmt.Errorf(
			"execution_date: %w",
			err,
		))
	}
	client, status := lookupClient(handle, outError)
	if status != C.FINTS_OK {
		return status
	}
	result, err := client.SEPACreditTransfer(
		context.Background(),
		fints.SEPACreditTransfer{
			Debtor:            accountFromC(&transfer.debtor),
			DebtorName:        goString(transfer.debtor_name),
			CreditorName:      goString(transfer.creditor_name),
			CreditorIBAN:      goString(transfer.creditor_iban),
			CreditorBIC:       goString(transfer.creditor_bic),
			Amount:            int64(transfer.amount),
			Currency:          goString(transfer.currency),
			Purpose:           goString(transfer.purpose),
			EndToEndReference: goString(transfer.end_to_end_reference),
			ExecutionDate:     executionDate,
		},
	)
	if err != nil {
		return failOperation(outError, err)
	}
	*out = transferResultToC(result)
	return C.FINTS_OK
}

// FintsErrorFree releases an error allocated by the library.
//
//export FintsErrorFree
func FintsErrorFree(value *C.FintsError) {
	if value == nil {
		return
	}
	freeCString(value.message)
	C.free(unsafe.Pointer(value))
}

// FintsAccountArrayFree releases an account array and all nested strings.
//
//export FintsAccountArrayFree
func FintsAccountArrayFree(value *C.FintsAccountArray) {
	if value == nil {
		return
	}
	for index := range unsafe.Slice(value.items, int(value.length)) {
		freeAccount(&unsafe.Slice(value.items, int(value.length))[index])
	}
	C.free(unsafe.Pointer(value.items))
	*value = C.FintsAccountArray{}
}

// FintsTransactionArrayFree releases a transaction array.
//
//export FintsTransactionArrayFree
func FintsTransactionArrayFree(value *C.FintsTransactionArray) {
	if value == nil {
		return
	}
	items := unsafe.Slice(value.items, int(value.length))
	for index := range items {
		freeTransaction(&items[index])
	}
	C.free(unsafe.Pointer(value.items))
	*value = C.FintsTransactionArray{}
}

// FintsCapabilityArrayFree releases a capability array.
//
//export FintsCapabilityArrayFree
func FintsCapabilityArrayFree(value *C.FintsCapabilityArray) {
	if value == nil {
		return
	}
	items := unsafe.Slice(value.items, int(value.length))
	for index := range items {
		freeCString(items[index].operation)
		C.free(unsafe.Pointer(items[index].versions))
		freeCStringArray(items[index].formats, items[index].format_count)
	}
	C.free(unsafe.Pointer(value.items))
	*value = C.FintsCapabilityArray{}
}

// FintsTANMethodArrayFree releases a TAN-method array.
//
//export FintsTANMethodArrayFree
func FintsTANMethodArrayFree(value *C.FintsTANMethodArray) {
	if value == nil {
		return
	}
	items := unsafe.Slice(value.items, int(value.length))
	for index := range items {
		freeCString(items[index].security_function)
		freeCString(items[index].name)
		freeCString(items[index].tan_process)
	}
	C.free(unsafe.Pointer(value.items))
	*value = C.FintsTANMethodArray{}
}

// FintsTANMediumArrayFree releases a TAN-medium array.
//
//export FintsTANMediumArrayFree
func FintsTANMediumArrayFree(value *C.FintsTANMediumArray) {
	if value == nil {
		return
	}
	items := unsafe.Slice(value.items, int(value.length))
	for index := range items {
		freeCString(items[index].name)
		freeCString(items[index].class_name)
		freeCString(items[index].status)
		freeCString(items[index].security_function)
		freeCString(items[index].mobile_number)
	}
	C.free(unsafe.Pointer(value.items))
	*value = C.FintsTANMediumArray{}
}

// FintsTransferResultFree releases a transfer result.
//
//export FintsTransferResultFree
func FintsTransferResultFree(value *C.FintsTransferResult) {
	if value == nil {
		return
	}
	freeCString(value.operation)
	freeCString(value.pain_descriptor)
	items := unsafe.Slice(value.responses, int(value.response_count))
	for index := range items {
		freeResponseMessage(&items[index])
	}
	C.free(unsafe.Pointer(value.responses))
	*value = C.FintsTransferResult{}
}

func main() {}

func configFromC(input *C.FintsConfig) (fints.Config, error) {
	compatibility := &input.compatibility
	config := fints.Config{
		Endpoint:         goString(input.endpoint),
		CountryCode:      goString(input.country_code),
		BankCode:         goString(input.bank_code),
		UserID:           goString(input.user_id),
		CustomerID:       goString(input.customer_id),
		PIN:              goString(input.pin),
		ProductID:        goString(input.product_id),
		ProductVersion:   goString(input.product_version),
		SystemID:         goString(input.system_id),
		SecurityFunction: goString(input.security_function),
		TANMedium:        goString(input.tan_medium),
		HKTANVersion:     int(input.hktan_version),
		AutoPoll:         input.auto_poll != 0,
		MaxPolls:         int(input.max_polls),
		AllowHTTP:        input.allow_http != 0,
		Compatibility: fints.CompatibilityOptions{
			SkipTransactionSEPAAccountDiscovery: compatibility.skip_transaction_sepa_account_discovery != 0,
			TransactionAccountDefaults:          accountFromC(&compatibility.transaction_account_defaults),
			ForceTANForOperations:               goStringArray(compatibility.force_tan_for_operations, compatibility.force_tan_for_operation_count),
			PreferredSecurityFunctions:          goStringArray(compatibility.preferred_security_functions, compatibility.preferred_security_function_count),
			TANMediumRequiredForFunctions:       goStringArray(compatibility.tan_medium_required_for_functions, compatibility.tan_medium_required_for_function_count),
			DecoupledPollProcess:                goString(compatibility.decoupled_poll_process),
		},
	}
	if input.poll_interval_ms > 0 {
		config.PollInterval = time.Duration(input.poll_interval_ms) * time.Millisecond
	}
	if input.timeout_ms > 0 {
		config.Timeout = time.Duration(input.timeout_ms) * time.Millisecond
	}
	if input.tan_callback != nil {
		callback := input.tan_callback
		userData := input.tan_user_data
		config.TANHandler = func(
			_ context.Context,
			challenge fints.TANChallenge,
		) (string, error) {
			cChallenge := challengeToC(challenge)
			defer freeChallenge(&cChallenge)
			value := C.fints_call_tan_callback(callback, &cChallenge, userData)
			if value == nil {
				if challenge.Decoupled {
					return "", nil
				}
				return "", errors.New("C TAN callback returned NULL for a manual TAN")
			}
			return C.GoString(value), nil
		}
	}
	return config, nil
}

func lookupClient(
	handle C.FintsClientHandle,
	outError **C.FintsError,
) (*fints.Client, C.int32_t) {
	value, ok := clientRegistry.Load(uint64(handle))
	if !ok {
		return nil, fail(outError, C.FINTS_ERROR_INVALID_HANDLE, fmt.Errorf(
			"unknown client handle %d",
			uint64(handle),
		))
	}
	return value.(*fints.Client), C.FINTS_OK
}

func failOperation(outError **C.FintsError, err error) C.int32_t {
	var bankError *fints.BankError
	if errors.As(err, &bankError) {
		return fail(outError, C.FINTS_ERROR_BANK, err)
	}
	if errors.Is(err, fints.ErrTANRequired) {
		return fail(outError, C.FINTS_ERROR_TAN_REQUIRED, err)
	}
	return fail(outError, C.FINTS_ERROR_OPERATION, err)
}

func fail(outError **C.FintsError, code C.int32_t, err error) C.int32_t {
	if outError == nil {
		return code
	}
	value := (*C.FintsError)(C.calloc(1, C.size_t(unsafe.Sizeof(C.FintsError{}))))
	if value == nil {
		return code
	}
	value.code = code
	value.message = C.CString(err.Error())
	*outError = value
	return code
}

func clearError(outError **C.FintsError) {
	if outError != nil {
		*outError = nil
	}
}

func goString(value *C.char) string {
	if value == nil {
		return ""
	}
	return C.GoString(value)
}

func goStringArray(values **C.char, count C.size_t) []string {
	if values == nil || count == 0 {
		return nil
	}
	input := unsafe.Slice(values, int(count))
	result := make([]string, 0, len(input))
	for _, value := range input {
		result = append(result, goString(value))
	}
	return result
}

func parseDate(value *C.char) (time.Time, error) {
	text := goString(value)
	if text == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.DateOnly, text)
}

func accountFromC(value *C.FintsAccount) fints.Account {
	if value == nil {
		return fints.Account{}
	}
	return fints.Account{
		IBAN:          goString(value.iban),
		BIC:           goString(value.bic),
		AccountNumber: goString(value.account_number),
		Subaccount:    goString(value.subaccount),
		CountryCode:   goString(value.country_code),
		BankCode:      goString(value.bank_code),
		Currency:      goString(value.currency),
		Type:          goString(value._type),
		OwnerName:     goString(value.owner_name),
		ProductName:   goString(value.product_name),
	}
}

func accountArrayToC(values []fints.Account) C.FintsAccountArray {
	result := C.FintsAccountArray{length: C.size_t(len(values))}
	if len(values) == 0 {
		return result
	}
	result.items = (*C.FintsAccount)(C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(C.FintsAccount{})),
	))
	items := unsafe.Slice(result.items, len(values))
	for index := range values {
		fillAccount(&items[index], values[index])
	}
	return result
}

func fillAccount(output *C.FintsAccount, value fints.Account) {
	output.iban = C.CString(value.IBAN)
	output.bic = C.CString(value.BIC)
	output.account_number = C.CString(value.AccountNumber)
	output.subaccount = C.CString(value.Subaccount)
	output.country_code = C.CString(value.CountryCode)
	output.bank_code = C.CString(value.BankCode)
	output.currency = C.CString(value.Currency)
	output._type = C.CString(value.Type)
	output.owner_name = C.CString(value.OwnerName)
	output.product_name = C.CString(value.ProductName)
	var operations []string
	for operation, supported := range value.SupportedOperations {
		if supported {
			operations = append(operations, operation)
		}
	}
	sort.Strings(operations)
	output.supported_operations, output.supported_operation_count = cStringArray(operations)
}

func transactionArrayToC(values []fints.Transaction) C.FintsTransactionArray {
	result := C.FintsTransactionArray{length: C.size_t(len(values))}
	if len(values) == 0 {
		return result
	}
	result.items = (*C.FintsTransaction)(C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(C.FintsTransaction{})),
	))
	items := unsafe.Slice(result.items, len(values))
	for index, value := range values {
		items[index].booking_date = C.CString(value.BookingDate)
		items[index].value_date = C.CString(value.ValueDate)
		items[index].amount = C.int64_t(value.Amount)
		items[index].currency = C.CString(value.Currency)
		items[index]._type = C.CString(value.Type)
		items[index].customer_reference = C.CString(value.CustomerReference)
		items[index].bank_reference = C.CString(value.BankReference)
		items[index].counterparty_name = C.CString(value.CounterpartyName)
		items[index].counterparty_iban = C.CString(value.CounterpartyIBAN)
		items[index].counterparty_bank_id = C.CString(value.CounterpartyBankID)
		items[index].description = C.CString(value.Description)
	}
	return result
}

func capabilityArrayToC(values []fints.Capability) C.FintsCapabilityArray {
	result := C.FintsCapabilityArray{length: C.size_t(len(values))}
	if len(values) == 0 {
		return result
	}
	result.items = (*C.FintsCapability)(C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(C.FintsCapability{})),
	))
	items := unsafe.Slice(result.items, len(values))
	for index, value := range values {
		items[index].operation = C.CString(value.Operation)
		items[index].versions, items[index].version_count = cIntArray(value.Versions)
		items[index].formats, items[index].format_count = cStringArray(value.Formats)
		items[index].tan_required = cBool(value.TANRequired)
		items[index].max_days = C.int32_t(value.MaxDays)
	}
	return result
}

func tanMethodArrayToC(values []fints.TANMethod) C.FintsTANMethodArray {
	result := C.FintsTANMethodArray{length: C.size_t(len(values))}
	if len(values) == 0 {
		return result
	}
	result.items = (*C.FintsTANMethod)(C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(C.FintsTANMethod{})),
	))
	items := unsafe.Slice(result.items, len(values))
	for index, value := range values {
		items[index].security_function = C.CString(value.SecurityFunction)
		items[index].name = C.CString(value.Name)
		items[index].hktan_version = C.int32_t(value.HKTANVersion)
		items[index].tan_process = C.CString(value.TANProcess)
		items[index].medium_required = cBool(value.MediumRequired)
	}
	return result
}

func tanMediumArrayToC(values []fints.TANMedium) C.FintsTANMediumArray {
	result := C.FintsTANMediumArray{length: C.size_t(len(values))}
	if len(values) == 0 {
		return result
	}
	result.items = (*C.FintsTANMedium)(C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(C.FintsTANMedium{})),
	))
	items := unsafe.Slice(result.items, len(values))
	for index, value := range values {
		items[index].name = C.CString(value.Name)
		items[index].class_name = C.CString(value.Class)
		items[index].status = C.CString(value.Status)
		items[index].security_function = C.CString(value.SecurityFunction)
		items[index].mobile_number = C.CString(value.MobileNumber)
	}
	return result
}

func transferResultToC(value *fints.TransferResult) C.FintsTransferResult {
	result := C.FintsTransferResult{
		accepted:        cBool(value.Accepted),
		operation:       C.CString(value.Operation),
		pain_descriptor: C.CString(value.PainDescriptor),
		response_count:  C.size_t(len(value.Responses)),
	}
	if len(value.Responses) == 0 {
		return result
	}
	result.responses = (*C.FintsResponseMessage)(C.calloc(
		C.size_t(len(value.Responses)),
		C.size_t(unsafe.Sizeof(C.FintsResponseMessage{})),
	))
	items := unsafe.Slice(result.responses, len(value.Responses))
	for index, response := range value.Responses {
		items[index].code = C.CString(response.Code)
		items[index].message = C.CString(response.Message)
		items[index].segment = C.int32_t(response.Segment)
		items[index].reference_element = C.CString(response.ReferenceElement)
		items[index].params, items[index].param_count = cStringArray(response.Params)
	}
	return result
}

func challengeToC(value fints.TANChallenge) C.FintsTANChallenge {
	return C.FintsTANChallenge{
		method:         C.CString(value.Method),
		medium:         C.CString(value.Medium),
		process:        C.CString(value.Process),
		hktan_version:  C.int32_t(value.HKTANVersion),
		task_reference: C.CString(value.TaskReference),
		challenge:      C.CString(value.Challenge),
		decoupled:      cBool(value.Decoupled),
		poll_attempt:   C.int32_t(value.PollAttempt),
	}
}

func cStringArray(values []string) (**C.char, C.size_t) {
	if len(values) == 0 {
		return nil, 0
	}
	pointer := C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(uintptr(0))),
	)
	items := unsafe.Slice((**C.char)(pointer), len(values))
	for index, value := range values {
		items[index] = C.CString(value)
	}
	return (**C.char)(pointer), C.size_t(len(values))
}

func cIntArray(values []int) (*C.int32_t, C.size_t) {
	if len(values) == 0 {
		return nil, 0
	}
	pointer := (*C.int32_t)(C.calloc(
		C.size_t(len(values)),
		C.size_t(unsafe.Sizeof(C.int32_t(0))),
	))
	items := unsafe.Slice(pointer, len(values))
	for index, value := range values {
		items[index] = C.int32_t(value)
	}
	return pointer, C.size_t(len(values))
}

func cBool(value bool) C.int32_t {
	if value {
		return 1
	}
	return 0
}

func freeAccount(value *C.FintsAccount) {
	freeCString(value.iban)
	freeCString(value.bic)
	freeCString(value.account_number)
	freeCString(value.subaccount)
	freeCString(value.country_code)
	freeCString(value.bank_code)
	freeCString(value.currency)
	freeCString(value._type)
	freeCString(value.owner_name)
	freeCString(value.product_name)
	freeCStringArray(value.supported_operations, value.supported_operation_count)
}

func freeTransaction(value *C.FintsTransaction) {
	freeCString(value.booking_date)
	freeCString(value.value_date)
	freeCString(value.currency)
	freeCString(value._type)
	freeCString(value.customer_reference)
	freeCString(value.bank_reference)
	freeCString(value.counterparty_name)
	freeCString(value.counterparty_iban)
	freeCString(value.counterparty_bank_id)
	freeCString(value.description)
}

func freeResponseMessage(value *C.FintsResponseMessage) {
	freeCString(value.code)
	freeCString(value.message)
	freeCString(value.reference_element)
	freeCStringArray(value.params, value.param_count)
}

func freeChallenge(value *C.FintsTANChallenge) {
	freeCString(value.method)
	freeCString(value.medium)
	freeCString(value.process)
	freeCString(value.task_reference)
	freeCString(value.challenge)
}

func freeCStringArray(values **C.char, count C.size_t) {
	if values == nil {
		return
	}
	for _, value := range unsafe.Slice(values, int(count)) {
		freeCString(value)
	}
	C.free(unsafe.Pointer(values))
}

func freeCString(value *C.char) {
	if value != nil {
		C.free(unsafe.Pointer(value))
	}
}
