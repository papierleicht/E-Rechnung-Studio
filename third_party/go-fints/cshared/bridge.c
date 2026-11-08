#include <stddef.h>
#include <stdint.h>

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

char* fints_call_tan_callback(
	FintsTANCallback callback,
	const FintsTANChallenge *challenge,
	void *user_data
) {
	if (callback == NULL) {
		return NULL;
	}
	return callback(challenge, user_data);
}
