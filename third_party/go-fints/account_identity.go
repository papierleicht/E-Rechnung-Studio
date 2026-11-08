package fints

import (
	"errors"
	"regexp"
	"strings"
)

var ibanIdentifierPattern = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
var nationalIdentifierPattern = regexp.MustCompile(`^[0-9]+$`)

func accountIdentityError() error {
	return errors.New("bank transaction response identifies a different account")
}

// Missing identifiers in old response formats are tolerated. Explicit account
// identifiers must agree with the requested account before any data is returned.
func checkResponseAccount(requested, returned Account) error {
	requested = normalizeAccount(requested)
	returnedIBAN := normalizeIBAN(returned.IBAN)
	if returnedIBAN != "" && requested.IBAN != "" && returnedIBAN != requested.IBAN {
		return accountIdentityError()
	}
	// A German IBAN unambiguously encodes BLZ and national account number. Do not
	// require a checksum here: a malformed returned identifier must not evade the
	// contradiction check by preventing normalization.
	if len(returnedIBAN) == 22 && strings.HasPrefix(returnedIBAN, "DE") {
		if returned.BankCode == "" {
			returned.BankCode = returnedIBAN[4:12]
		}
		if returned.AccountNumber == "" {
			returned.AccountNumber = returnedIBAN[12:]
		}
	}
	if returned.BankCode != "" && requested.BankCode != "" && strings.TrimSpace(returned.BankCode) != requested.BankCode {
		return accountIdentityError()
	}
	if returned.AccountNumber != "" && requested.AccountNumber != "" {
		left, right := strings.TrimSpace(returned.AccountNumber), requested.AccountNumber
		if nationalIdentifierPattern.MatchString(left) && nationalIdentifierPattern.MatchString(right) {
			left = strings.TrimLeft(left, "0")
			right = strings.TrimLeft(right, "0")
		}
		if left != right {
			return accountIdentityError()
		}
	}
	if returned.Subaccount != "" && requested.Subaccount != "" && returned.Subaccount != requested.Subaccount {
		return accountIdentityError()
	}
	return nil
}

func checkCAMTResponseAccounts(response []segment, requested Account) error {
	for _, s := range findSegments(response, "HICAZ") {
		returned := Account{IBAN: component(s, 0, 0), AccountNumber: component(s, 0, 2), Subaccount: component(s, 0, 3), BankCode: component(s, 0, 5)}
		if err := checkResponseAccount(requested, returned); err != nil {
			return err
		}
	}
	return nil
}

// The MT940 :25: field may carry an IBAN, a national account identifier or a
// BLZ/account pair. Unknown proprietary identifiers cannot safely be inferred.
func checkMT940AccountIdentifier(identifier string, requested Account) error {
	identifier = strings.TrimSpace(strings.SplitN(identifier, "\n", 2)[0])
	identifier = strings.TrimPrefix(identifier, "/")
	compact := normalizeIBAN(identifier)
	if ibanIdentifierPattern.MatchString(compact) {
		return checkResponseAccount(requested, Account{IBAN: compact})
	}
	if bank, number, ok := strings.Cut(identifier, "/"); ok && len(bank) == 8 && nationalIdentifierPattern.MatchString(bank) {
		returned := Account{BankCode: bank}
		if nationalIdentifierPattern.MatchString(strings.TrimSpace(number)) {
			returned.AccountNumber = strings.TrimSpace(number)
		}
		return checkResponseAccount(requested, returned)
	}
	if nationalIdentifierPattern.MatchString(identifier) {
		return checkResponseAccount(requested, Account{AccountNumber: identifier})
	}
	return nil
}
