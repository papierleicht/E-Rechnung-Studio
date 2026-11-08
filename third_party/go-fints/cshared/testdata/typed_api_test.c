#include "libfints.h"

#include <assert.h>
#include <stddef.h>
#include <string.h>

static char *tan_callback(
	const FintsTANChallenge *challenge,
	void *user_data
) {
	(void)challenge;
	(void)user_data;
	return NULL;
}

int main(void) {
	assert(FintsABIVersion() == 1);

	FintsClientHandle handle = 0;
	FintsError *error = NULL;
	int32_t status = FintsClientCreate(NULL, &handle, &error);
	assert(status == FINTS_ERROR_INVALID_ARGUMENT);
	assert(handle == 0);
	assert(error != NULL);
	assert(error->code == FINTS_ERROR_INVALID_ARGUMENT);
	assert(strstr(error->message, "config") != NULL);
	FintsErrorFree(error);

	FintsConfig config = {0};
	config.tan_callback = tan_callback;
	error = NULL;
	status = FintsClientCreate(&config, &handle, &error);
	assert(status == FINTS_ERROR_INVALID_ARGUMENT);
	assert(error != NULL);
	assert(strstr(error->message, "endpoint") != NULL);
	FintsErrorFree(error);

	FintsAccountArray accounts = {0};
	error = NULL;
	status = FintsAccounts(999999, &accounts, &error);
	assert(status == FINTS_ERROR_INVALID_HANDLE);
	assert(accounts.items == NULL);
	assert(accounts.length == 0);
	assert(error != NULL);
	FintsErrorFree(error);
	FintsAccountArrayFree(&accounts);

	FintsTransactionArray transactions = {0};
	FintsTransactionArrayFree(&transactions);
	FintsCapabilityArray capabilities = {0};
	FintsCapabilityArrayFree(&capabilities);
	FintsTANMethodArray methods = {0};
	FintsTANMethodArrayFree(&methods);
	FintsTANMediumArray media = {0};
	FintsTANMediumArrayFree(&media);
	FintsTransferResult transfer = {0};
	FintsTransferResultFree(&transfer);

	return 0;
}
